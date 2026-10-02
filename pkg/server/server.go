// Package server is the trimproof HTTP gateway (spec §3): routing,
// identity, policy lookup, representation gates, upstream forwarding and
// Tier 1 validation.
package server

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/AkashAgarwalInd/trimproof/pkg/codec"
	"github.com/AkashAgarwalInd/trimproof/pkg/ir"
	"github.com/AkashAgarwalInd/trimproof/pkg/policy"
	"github.com/AkashAgarwalInd/trimproof/pkg/provider"
	"github.com/AkashAgarwalInd/trimproof/pkg/provider/anthropic"
	"github.com/AkashAgarwalInd/trimproof/pkg/provider/openai"
	"github.com/AkashAgarwalInd/trimproof/pkg/tokens"
	"github.com/AkashAgarwalInd/trimproof/pkg/validator"
)

// MaxBodyBytes bounds request bodies read by the gateway.
const MaxBodyBytes = 32 << 20

// Exchange is what observers (shadow evaluator, audit) see after a request
// has been served. Observers must not block.
type Exchange struct {
	Start      time.Time
	Latency    time.Duration
	Route      *Route
	Sec        validator.SecurityContext
	Adapter    provider.Adapter
	Upstream   string // full upstream URL
	Header     http.Header
	Original   *ir.Request // as received
	Decision   *policy.Decision
	SentBody   []byte // the body actually sent on the final attempt
	Sent       string // "original" | "encoded"
	FellBack   bool
	Status     int
	Response   *ir.Response // nil for pass-through/streamed responses
	Tier1      *validator.Result
	ParseError error
}

// Observer receives completed exchanges.
type Observer interface {
	Observe(ctx context.Context, x *Exchange)
}

// Config configures a Server.
type Config struct {
	Registry      *Registry
	AnthropicBase string // e.g. https://api.anthropic.com
	OpenAIBase    string // e.g. https://api.openai.com/v1
	IdentityMode  IdentityMode
	IdentityKey   []byte
	// RequireIdentity rejects requests without identity. When false,
	// anonymous requests are served on the "*" tenant and fail Tier 1 authz.
	RequireIdentity bool
	Estimator       tokens.Estimator
	HTTP            *http.Client
	Observers       []Observer
	Log             *slog.Logger
}

// Server is the gateway handler.
type Server struct {
	cfg Config
	mux *http.ServeMux
}

// New builds a Server.
func New(cfg Config) *Server {
	if cfg.HTTP == nil {
		cfg.HTTP = &http.Client{Timeout: 10 * time.Minute}
	}
	if cfg.Estimator == nil {
		cfg.Estimator = tokens.NewCalibrated(nil)
	}
	if cfg.Log == nil {
		cfg.Log = slog.Default()
	}
	if cfg.Registry == nil {
		cfg.Registry = NewRegistry()
	}
	s := &Server{cfg: cfg, mux: http.NewServeMux()}
	s.mux.HandleFunc("POST /anthropic/v1/messages", func(w http.ResponseWriter, r *http.Request) {
		s.handle(w, r, anthropic.Adapter{}, strings.TrimRight(cfg.AnthropicBase, "/")+"/v1/messages")
	})
	s.mux.HandleFunc("POST /openai/v1/chat/completions", func(w http.ResponseWriter, r *http.Request) {
		s.handle(w, r, openai.Adapter{}, strings.TrimRight(cfg.OpenAIBase, "/")+"/chat/completions")
	})
	s.mux.HandleFunc("/anthropic/", func(w http.ResponseWriter, r *http.Request) {
		s.proxy(w, r, strings.TrimRight(cfg.AnthropicBase, "/")+strings.TrimPrefix(r.URL.Path, "/anthropic"))
	})
	s.mux.HandleFunc("/openai/v1/", func(w http.ResponseWriter, r *http.Request) {
		s.proxy(w, r, strings.TrimRight(cfg.OpenAIBase, "/")+strings.TrimPrefix(r.URL.Path, "/openai/v1"))
	})
	s.mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) { w.Write([]byte("ok")) })
	return s
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) { s.mux.ServeHTTP(w, r) }

// Registry exposes the policy registry (used by the promotion engine).
func (s *Server) Registry() *Registry { return s.cfg.Registry }

func (s *Server) handle(w http.ResponseWriter, r *http.Request, ad provider.Adapter, upstream string) {
	x := &Exchange{Start: time.Now(), Adapter: ad, Upstream: upstream, Sent: "original"}
	defer func() {
		x.Latency = time.Since(x.Start)
		for _, o := range s.cfg.Observers {
			o.Observe(context.WithoutCancel(r.Context()), x)
		}
	}()

	body, err := io.ReadAll(io.LimitReader(r.Body, MaxBodyBytes+1))
	if err != nil || len(body) > MaxBodyBytes {
		writeError(w, http.StatusRequestEntityTooLarge, "request_too_large", "request body exceeds limit", nil)
		x.Status = http.StatusRequestEntityTooLarge
		return
	}
	sec, idErr := Identify(r, s.cfg.IdentityMode, s.cfg.IdentityKey, time.Now())
	if idErr != nil && s.cfg.RequireIdentity {
		writeError(w, http.StatusUnauthorized, "identity_required", idErr.Error(), nil)
		x.Status = http.StatusUnauthorized
		return
	}
	x.Sec = sec
	tenant := sec.TenantID
	if tenant == "" {
		tenant = "*"
	}
	x.Route = s.cfg.Registry.Lookup(tenant, r.Header.Get(HeaderRoute))
	x.Header = upstreamHeaders(r.Header)
	pol := x.Route.Policy

	req, err := ad.ParseRequest(body)
	if err != nil {
		// Unparseable: never transform, never validate; forward verbatim and
		// let the provider answer.
		x.ParseError = err
		x.SentBody = body
		x.Status = s.forwardRaw(w, r.Context(), upstream, x.Header, body)
		return
	}
	x.Original = req
	for _, loc := range parseMarks(r.Header.Get(HeaderData)) {
		req.Mark(loc)
	}

	// Gates (spec §5).
	blocks := req.DataBlocks()
	payloads := make([][]byte, len(blocks))
	for i, b := range blocks {
		payloads[i] = b.Data
	}
	d, err := policy.Decide(pol, payloads, s.cfg.Estimator, req.Model)
	if err != nil {
		s.cfg.Log.Error("gate decision failed", "err", err)
		d = &policy.Decision{}
	}
	x.Decision = d

	sendBody := body
	if d.ApplyToProduction && d.Any() {
		enc, err := s.encodedBody(ad, req, d)
		if err != nil {
			s.cfg.Log.Error("render failed; sending original", "err", err)
		} else {
			sendBody, x.Sent = enc, "encoded"
		}
	}
	w.Header().Set("X-Trimproof-Representation", representation(x.Sent, d))

	// Streaming without Tier 1: pass through as it arrives.
	if req.Stream && x.Route.Validator == nil {
		x.SentBody = sendBody
		x.Status = s.forwardStream(w, r.Context(), upstream, x.Header, sendBody)
		return
	}

	status, hdr, respBody, err := s.roundTrip(r.Context(), upstream, x.Header, sendBody)
	x.SentBody = sendBody
	if err != nil {
		writeError(w, http.StatusBadGateway, "upstream_error", err.Error(), nil)
		x.Status = http.StatusBadGateway
		return
	}
	// Provider errors pass through untouched and never trigger a
	// representation fallback (spec invariant 12).
	if status != http.StatusOK || x.Route.Validator == nil {
		if status == http.StatusOK && !req.Stream {
			x.Response, _ = ad.ParseResponse(req, respBody)
		}
		x.Status = status
		writeUpstream(w, status, hdr, respBody)
		return
	}

	// Tier 1 (spec §7).
	resp, res := s.validate(r.Context(), ad, req, respBody, x.Route.Validator, sec)
	if !res.OK && x.Sent == "encoded" && pol.AllowFallbackRetry {
		x.FellBack, x.Sent = true, "original"
		w.Header().Set("X-Trimproof-Representation", "json (fallback)")
		status, hdr, respBody, err = s.roundTrip(r.Context(), upstream, x.Header, body)
		x.SentBody = body
		if err != nil {
			writeError(w, http.StatusBadGateway, "upstream_error", err.Error(), nil)
			x.Status = http.StatusBadGateway
			return
		}
		if status != http.StatusOK {
			x.Status = status
			writeUpstream(w, status, hdr, respBody)
			return
		}
		resp, res = s.validate(r.Context(), ad, req, respBody, x.Route.Validator, sec)
	}
	x.Response, x.Tier1 = resp, &res
	if !res.OK {
		x.Status = http.StatusUnprocessableEntity
		writeError(w, http.StatusUnprocessableEntity, "trimproof_validation_failed",
			"the model response failed Tier 1 validation", res.Violations)
		return
	}
	x.Status = status
	writeUpstream(w, status, hdr, respBody)
}

func (s *Server) validate(ctx context.Context, ad provider.Adapter, req *ir.Request, respBody []byte, v *validator.Validator, sec validator.SecurityContext) (*ir.Response, validator.Result) {
	var resp *ir.Response
	var err error
	if req.Stream {
		sa, ok := ad.(provider.StreamAssembler)
		if !ok {
			return nil, validator.Result{Violations: []validator.Violation{{Step: validator.StepContract, Message: "streaming validation unsupported for provider"}}}
		}
		resp, err = sa.AssembleStream(req, respBody)
	} else {
		resp, err = ad.ParseResponse(req, respBody)
	}
	if err != nil {
		return nil, validator.Result{Violations: []validator.Violation{{Step: validator.StepContract, Message: "unparseable provider response: " + err.Error()}}}
	}
	return resp, v.Validate(ctx, req, resp, sec)
}

// encodedBody applies the decision's transforms to a copy of the request
// and renders it.
func (s *Server) encodedBody(ad provider.Adapter, req *ir.Request, d *policy.Decision) ([]byte, error) {
	return RenderWithDecision(ad, req, d)
}

// RenderWithDecision renders req with d's transforms and primer applied,
// leaving req unmodified. The shadow evaluator uses it for the codec arm.
func RenderWithDecision(ad provider.Adapter, req *ir.Request, d *policy.Decision) ([]byte, error) {
	cp := *req
	cp.Messages = make([]ir.Message, len(req.Messages))
	for i, m := range req.Messages {
		cp.Messages[i] = ir.Message{Role: m.Role, Blocks: append([]ir.Block(nil), m.Blocks...)}
	}
	c, ok := codec.Get(d.Codec)
	if !ok {
		return nil, fmt.Errorf("unknown codec %q", d.Codec)
	}
	for i, b := range cp.DataBlocks() {
		if i < len(d.Blocks) && d.Blocks[i].Transform {
			b.Transform = &ir.AppliedTransform{Codec: c.Name(), Version: c.Version(), Encoded: d.Blocks[i].Encoded}
		}
	}
	cp.Primer = c.Primer()
	return ad.RenderRequest(&cp)
}

func representation(sent string, d *policy.Decision) string {
	if sent == "encoded" {
		return fmt.Sprintf("%s; est_savings=%.3f", d.Codec, d.NetSavings)
	}
	return "json"
}

// upstreamHeaders copies client headers for forwarding, dropping hop-by-hop
// headers and all trimproof control/identity headers.
func upstreamHeaders(in http.Header) http.Header {
	out := http.Header{}
	for k, vs := range in {
		ck := http.CanonicalHeaderKey(k)
		switch {
		case strings.HasPrefix(ck, "X-Tp-"), strings.HasPrefix(ck, "X-Trimproof-"):
			continue
		}
		switch ck {
		case "Host", "Content-Length", "Connection", "Keep-Alive", "Proxy-Connection",
			"Transfer-Encoding", "Upgrade", "Te", "Trailer", "Accept-Encoding":
			continue
		}
		out[ck] = append([]string(nil), vs...)
	}
	return out
}

func (s *Server) newRequest(ctx context.Context, method, url string, hdr http.Header, body []byte) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, method, url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header = hdr.Clone()
	req.ContentLength = int64(len(body))
	return req, nil
}

func (s *Server) roundTrip(ctx context.Context, url string, hdr http.Header, body []byte) (int, http.Header, []byte, error) {
	req, err := s.newRequest(ctx, http.MethodPost, url, hdr, body)
	if err != nil {
		return 0, nil, nil, err
	}
	resp, err := s.cfg.HTTP.Do(req)
	if err != nil {
		return 0, nil, nil, err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	return resp.StatusCode, resp.Header, data, err
}

func (s *Server) forwardRaw(w http.ResponseWriter, ctx context.Context, url string, hdr http.Header, body []byte) int {
	status, h, data, err := s.roundTrip(ctx, url, hdr, body)
	if err != nil {
		writeError(w, http.StatusBadGateway, "upstream_error", err.Error(), nil)
		return http.StatusBadGateway
	}
	writeUpstream(w, status, h, data)
	return status
}

// forwardStream relays an SSE response chunk by chunk.
func (s *Server) forwardStream(w http.ResponseWriter, ctx context.Context, url string, hdr http.Header, body []byte) int {
	req, err := s.newRequest(ctx, http.MethodPost, url, hdr, body)
	if err != nil {
		writeError(w, http.StatusBadGateway, "upstream_error", err.Error(), nil)
		return http.StatusBadGateway
	}
	resp, err := s.cfg.HTTP.Do(req)
	if err != nil {
		writeError(w, http.StatusBadGateway, "upstream_error", err.Error(), nil)
		return http.StatusBadGateway
	}
	defer resp.Body.Close()
	copyHeaders(w.Header(), resp.Header)
	w.WriteHeader(resp.StatusCode)
	fl, _ := w.(http.Flusher)
	buf := make([]byte, 32<<10)
	for {
		n, err := resp.Body.Read(buf)
		if n > 0 {
			if _, werr := w.Write(buf[:n]); werr != nil {
				break
			}
			if fl != nil {
				fl.Flush()
			}
		}
		if err != nil {
			break
		}
	}
	return resp.StatusCode
}

// proxy forwards non-intercepted provider endpoints unchanged.
func (s *Server) proxy(w http.ResponseWriter, r *http.Request, url string) {
	body, err := io.ReadAll(io.LimitReader(r.Body, MaxBodyBytes))
	if err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", err.Error(), nil)
		return
	}
	if r.URL.RawQuery != "" {
		url += "?" + r.URL.RawQuery
	}
	req, err := s.newRequest(r.Context(), r.Method, url, upstreamHeaders(r.Header), body)
	if err != nil {
		writeError(w, http.StatusBadGateway, "upstream_error", err.Error(), nil)
		return
	}
	resp, err := s.cfg.HTTP.Do(req)
	if err != nil {
		writeError(w, http.StatusBadGateway, "upstream_error", err.Error(), nil)
		return
	}
	defer resp.Body.Close()
	copyHeaders(w.Header(), resp.Header)
	w.WriteHeader(resp.StatusCode)
	io.Copy(w, resp.Body)
}

func copyHeaders(dst, src http.Header) {
	for k, vs := range src {
		switch http.CanonicalHeaderKey(k) {
		case "Content-Length", "Content-Encoding", "Connection", "Transfer-Encoding":
			continue
		}
		dst[k] = append([]string(nil), vs...)
	}
}

func writeUpstream(w http.ResponseWriter, status int, hdr http.Header, body []byte) {
	copyHeaders(w.Header(), hdr)
	w.Header().Set("Content-Length", strconv.Itoa(len(body)))
	w.WriteHeader(status)
	w.Write(body)
}

func writeError(w http.ResponseWriter, status int, typ, msg string, violations []validator.Violation) {
	body, _ := json.Marshal(map[string]any{
		"type": "error",
		"error": map[string]any{
			"type":       typ,
			"message":    msg,
			"violations": violations,
		},
	})
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	w.Write(body)
}

// parseMarks parses "3,5:1" into locators (message 3 whole string content;
// message 5 part 1).
func parseMarks(h string) []ir.Locator {
	var out []ir.Locator
	for _, f := range strings.Split(h, ",") {
		f = strings.TrimSpace(f)
		if f == "" {
			continue
		}
		m, p, hasPart := strings.Cut(f, ":")
		mi, err := strconv.Atoi(m)
		if err != nil {
			continue
		}
		loc := ir.Locator{Message: mi, Part: -1}
		if hasPart {
			if loc.Part, err = strconv.Atoi(p); err != nil {
				continue
			}
		}
		out = append(out, loc)
	}
	return out
}
