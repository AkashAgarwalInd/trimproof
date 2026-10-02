// Command gateway runs the trimproof LLM gateway.
//
//	gateway -policies policies.json -listen :8080
//
// Clients point their SDK base URL at the gateway:
//
//	Anthropic: http://gateway:8080/anthropic   (POST /v1/messages is intercepted)
//	OpenAI:    http://gateway:8080/openai/v1   (POST /chat/completions is intercepted)
//
// and send X-Trimproof-Route plus identity (X-TP-Identity JWT) from
// trusted ingress. Provider API keys pass through from the client.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"

	"github.com/AkashAgarwalInd/trimproof/pkg/audit"
	"github.com/AkashAgarwalInd/trimproof/pkg/calibrate"
	_ "github.com/AkashAgarwalInd/trimproof/pkg/codec/tabular"
	_ "github.com/AkashAgarwalInd/trimproof/pkg/codec/toon"
	_ "github.com/AkashAgarwalInd/trimproof/pkg/codec/toonx"
	"github.com/AkashAgarwalInd/trimproof/pkg/eval"
	"github.com/AkashAgarwalInd/trimproof/pkg/server"
	"github.com/AkashAgarwalInd/trimproof/pkg/tokens"
)

// version is set at build time with -ldflags "-X main.version=...".
var version = "dev"

func main() {
	listen := flag.String("listen", ":8080", "listen address")
	policies := flag.String("policies", "policies.json", "route policy file")
	anthropicBase := flag.String("anthropic-base", "https://api.anthropic.com", "Anthropic API base URL")
	openaiBase := flag.String("openai-base", envOr("OPENAI_API_BASE", "https://api.openai.com/v1"), "OpenAI-compatible base URL (including /v1)")
	identityMode := flag.String("identity-mode", string(server.IdentityJWT), "jwt-hs256 | trusted-headers")
	identityKeyEnv := flag.String("identity-key-env", "TP_IDENTITY_KEY", "env var holding the HS256 identity key")
	requireIdentity := flag.Bool("require-identity", true, "reject requests without identity")
	pairsFile := flag.String("pairs-file", "eval-pairs.jsonl", "shadow evaluation pairs (JSONL)")
	auditFile := flag.String("audit-file", "audit.jsonl", "audit log (JSONL); empty disables")
	transitionsFile := flag.String("transitions-file", "transitions.jsonl", "promotion transitions (JSONL)")
	evalRPS := flag.Float64("eval-rps", 1, "per-provider evaluation request rate")
	evalInFlight := flag.Int("eval-max-inflight", 6, "per-provider concurrent evaluation requests (each sample runs 3)")
	calibrateEvery := flag.Duration("calibrate-interval", 15*time.Minute, "per Claude model, how often to correct token estimates with Anthropic's free count_tokens endpoint (client credentials); 0 disables")
	maxBody := flag.Int64("max-body-bytes", server.MaxBodyBytes, "maximum request body size")
	upstreamTimeout := flag.Duration("upstream-timeout", server.DefaultUpstreamTimeout, "limit for a buffered upstream call, and for the first byte of a streamed one")
	streamIdle := flag.Duration("stream-idle-timeout", server.DefaultStreamIdleTimeout, "maximum gap between chunks of a streamed response")
	shutdownTimeout := flag.Duration("shutdown-timeout", 30*time.Second, "how long to wait for in-flight requests on SIGTERM")
	showVersion := flag.Bool("version", false, "print version and exit")
	flag.Parse()
	if *showVersion {
		fmt.Println("trimproof-gateway", version)
		return
	}

	log := slog.New(slog.NewJSONHandler(os.Stderr, nil))
	slog.SetDefault(log)
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// OpenTelemetry metrics via OTLP/HTTP when OTEL_EXPORTER_OTLP_ENDPOINT is set.
	var mp *sdkmetric.MeterProvider
	if os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT") != "" {
		exp, err := otlpmetrichttp.New(ctx)
		if err != nil {
			fatal(log, "otlp exporter", err)
		}
		mp = sdkmetric.NewMeterProvider(sdkmetric.WithReader(sdkmetric.NewPeriodicReader(exp)))
		otel.SetMeterProvider(mp)
	}

	reg, err := server.LoadRegistry(*policies)
	if err != nil {
		fatal(log, "load policies", err)
	}
	restored, err := eval.ReplayTransitions(*transitionsFile, reg)
	if err != nil {
		fatal(log, "replay transitions", err)
	} else if len(restored) > 0 {
		log.Info("restored promotion state", "routes", len(restored))
	}
	metrics, err := audit.NewMetrics()
	if err != nil {
		fatal(log, "metrics", err)
	}
	store, err := eval.NewMemoryStore(*pairsFile)
	if err != nil {
		fatal(log, "pair store", err)
	}
	transitions := &appendLog{f: openAppend(log, *transitionsFile)}
	prom := &eval.Promoter{Registry: reg, Store: store, T: eval.DefaultThresholds(), OnTransition: func(tr eval.Transition) {
		metrics.Transition(tr)
		b, _ := json.Marshal(tr)
		if err := transitions.Write(append(b, '\n')); err != nil {
			log.Error("write transition", "err", err)
		}
		log.Info("route state changed", "route", tr.TenantID+"/"+tr.RouteID, "from", tr.From.String(), "to", tr.To.String(), "reason", tr.Reason)
	}}
	prom.Resume(restored)
	ev := eval.NewEvaluator(eval.Config{Store: store, Promoter: prom, RPS: *evalRPS, MaxInFlight: *evalInFlight, Log: log})
	evalDone := make(chan struct{})
	go func() { ev.Run(ctx); close(evalDone) }()

	observers := []server.Observer{metrics, prom, ev}
	estimator := tokens.NewCalibrated(nil)
	if *calibrateEvery > 0 {
		cal := calibrate.New(calibrate.Config{Estimator: estimator, AnthropicBase: *anthropicBase, Interval: *calibrateEvery, Log: log})
		go cal.Run(ctx)
		observers = append(observers, cal)
	}
	var aud *audit.Auditor
	var sink *audit.JSONLSink
	if *auditFile != "" {
		if sink, err = audit.NewJSONLFile(*auditFile); err != nil {
			fatal(log, "audit file", err)
		}
		aud = audit.New(audit.Config{Sinks: []audit.Sink{sink}, Log: log})
		observers = append(observers, aud)
	}

	gw := server.New(server.Config{
		Registry: reg, AnthropicBase: *anthropicBase, OpenAIBase: *openaiBase,
		IdentityMode: server.IdentityMode(*identityMode), IdentityKey: []byte(os.Getenv(*identityKeyEnv)),
		RequireIdentity: *requireIdentity, Estimator: estimator, Observers: observers, Log: log,
		MaxBodyBytes: *maxBody, UpstreamTimeout: *upstreamTimeout, StreamIdleTimeout: *streamIdle,
	})
	// No WriteTimeout: streamed responses are bounded by the gateway's
	// upstream and idle timeouts instead.
	srv := &http.Server{Addr: *listen, Handler: gw, ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout: 2 * time.Minute, IdleTimeout: 2 * time.Minute}
	serveErr := make(chan error, 1)
	go func() { serveErr <- srv.ListenAndServe() }()
	log.Info("trimproof gateway listening", "addr", *listen, "version", version, "policies", *policies)

	exit := 0
	select {
	case err := <-serveErr:
		log.Error("serve", "err", err)
		stop()
		exit = 1
	case <-ctx.Done():
		log.Info("shutting down", "timeout", shutdownTimeout.String())
		sctx, cancel := context.WithTimeout(context.Background(), *shutdownTimeout)
		if err := srv.Shutdown(sctx); err != nil {
			log.Warn("in-flight requests did not finish; closing connections", "err", err)
			srv.Close()
		}
		cancel()
	}

	// Stop in dependency order: nothing below may run before its writers
	// have stopped. The evaluator (its ctx is already done) finishes its
	// workers; audit drains its queue; then the files are flushed.
	<-evalDone
	if aud != nil {
		aud.Close()
		sink.Close()
	}
	store.Close()
	transitions.Close()
	if mp != nil {
		mp.Shutdown(context.Background())
	}
	log.Info("stopped", "eval_dropped", ev.Dropped())
	os.Exit(exit)
}

// appendLog is a JSONL file that tolerates writes after Close (a request
// still running past the shutdown deadline can trigger a transition).
type appendLog struct {
	mu sync.Mutex
	f  *os.File
}

func (l *appendLog) Write(b []byte) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.f == nil {
		return os.ErrClosed
	}
	_, err := l.f.Write(b)
	return err
}

func (l *appendLog) Close() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	f := l.f
	l.f = nil
	return f.Close()
}

func envOr(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func openAppend(log *slog.Logger, path string) *os.File {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		fatal(log, "open "+path, err)
	}
	return f
}

func fatal(log *slog.Logger, msg string, err error) {
	log.Error(msg, "err", err)
	os.Exit(1)
}
