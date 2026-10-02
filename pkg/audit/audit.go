package audit

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"log/slog"
	"math/rand/v2"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"github.com/AkashAgarwalInd/trimproof/pkg/codec"
	"github.com/AkashAgarwalInd/trimproof/pkg/ir"
	"github.com/AkashAgarwalInd/trimproof/pkg/policy"
	"github.com/AkashAgarwalInd/trimproof/pkg/server"
	"github.com/AkashAgarwalInd/trimproof/pkg/validator"
)

// BlockGate summarizes the gate outcome for one data block.
type BlockGate struct {
	Transform  bool   `json:"transform"`
	FailedGate string `json:"failed_gate,omitempty"`
	Reason     string `json:"reason,omitempty"`
	JSONTokens int    `json:"json_tokens,omitempty"`
	EncTokens  int    `json:"enc_tokens,omitempty"`
}

// Entry is one audit record.
type Entry struct {
	Time          time.Time `json:"time"`
	TenantID      string    `json:"tenant_id"`
	RouteID       string    `json:"route_id"`
	PolicyVersion string    `json:"policy_version,omitempty"`
	RouteState    string    `json:"route_state"`
	Subject       string    `json:"subject,omitempty"`
	Provider      string    `json:"provider"`
	Model         string    `json:"model,omitempty"`
	Status        int       `json:"status"`
	LatencyMS     int64     `json:"latency_ms"`

	// What was actually sent upstream on the final attempt (spec §9).
	Sent         string      `json:"sent"` // original | encoded
	FellBack     bool        `json:"fell_back,omitempty"`
	Codec        string      `json:"codec,omitempty"`
	CodecVersion string      `json:"codec_version,omitempty"`
	SentSHA256   string      `json:"sent_sha256"`
	EstSavings   float64     `json:"est_savings,omitempty"`
	Gates        []BlockGate `json:"gates,omitempty"`

	Usage *ir.Usage         `json:"usage,omitempty"`
	Tier1 *validator.Result `json:"tier1,omitempty"`

	// RequestRedacted is the ORIGINAL request, redacted by key, present only
	// when the route enables raw payload logging. Encoded payloads are never
	// logged raw: they are reproducible from this redacted original because
	// codecs are deterministic, and SentSHA256 identifies the exact bytes.
	RequestRedacted json.RawMessage `json:"request_redacted,omitempty"`
	ParseError      string          `json:"parse_error,omitempty"`
}

// Sink receives entries from the worker pool.
type Sink interface {
	Write(ctx context.Context, e *Entry) error
}

// JSONLSink appends entries to a writer, one JSON object per line.
type JSONLSink struct {
	mu sync.Mutex
	w  *bufio.Writer
	c  io.Closer
}

// NewJSONLFile opens (appending) a JSONL audit file.
func NewJSONLFile(path string) (*JSONLSink, error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return nil, err
	}
	return &JSONLSink{w: bufio.NewWriter(f), c: f}, nil
}

func (s *JSONLSink) Write(_ context.Context, e *Entry) error {
	b, err := json.Marshal(e)
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.w.Write(append(b, '\n'))
	return s.w.Flush()
}

func (s *JSONLSink) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.w.Flush()
	return s.c.Close()
}

// Config configures the audit pipeline.
type Config struct {
	Sinks     []Sink
	QueueSize int
	Workers   int
	Log       *slog.Logger
	Rand      func() float64
}

// Auditor is a server.Observer. It never blocks or alters traffic: when the
// queue is full the entry is dropped and counted (spec invariant 9).
type Auditor struct {
	cfg     Config
	queue   chan *server.Exchange
	dropped atomic.Int64
	written atomic.Int64
	wg      sync.WaitGroup
}

// New starts the worker pool.
func New(cfg Config) *Auditor {
	if cfg.QueueSize <= 0 {
		cfg.QueueSize = 1024
	}
	if cfg.Workers <= 0 {
		cfg.Workers = 2
	}
	if cfg.Log == nil {
		cfg.Log = slog.Default()
	}
	if cfg.Rand == nil {
		cfg.Rand = rand.Float64
	}
	a := &Auditor{cfg: cfg, queue: make(chan *server.Exchange, cfg.QueueSize)}
	for i := 0; i < cfg.Workers; i++ {
		a.wg.Add(1)
		go a.worker()
	}
	return a
}

// Dropped and Written expose pipeline counters.
func (a *Auditor) Dropped() int64 { return a.dropped.Load() }
func (a *Auditor) Written() int64 { return a.written.Load() }

// Observe samples by AuditSampleRate. Tier 1 failures, fallbacks and parse
// errors are always audited.
func (a *Auditor) Observe(_ context.Context, x *server.Exchange) {
	if x.Route == nil {
		return
	}
	always := x.FellBack || x.ParseError != nil || (x.Tier1 != nil && !x.Tier1.OK)
	if !always && a.cfg.Rand() >= x.Route.Policy.AuditSampleRate {
		return
	}
	select {
	case a.queue <- x:
	default:
		a.dropped.Add(1)
	}
}

// Close drains the queue and stops workers.
func (a *Auditor) Close() {
	close(a.queue)
	a.wg.Wait()
}

func (a *Auditor) worker() {
	defer a.wg.Done()
	for x := range a.queue {
		e := BuildEntry(x)
		for _, s := range a.cfg.Sinks {
			if err := s.Write(context.Background(), e); err != nil {
				a.cfg.Log.Warn("audit sink write failed", "err", err)
			}
		}
		a.written.Add(1)
	}
}

// BuildEntry converts an exchange to an audit entry, applying redaction.
func BuildEntry(x *server.Exchange) *Entry {
	pol := x.Route.Policy
	e := &Entry{
		Time: x.Start, TenantID: pol.TenantID, RouteID: pol.RouteID, PolicyVersion: pol.Version,
		RouteState: pol.State.String(), Subject: x.Sec.Subject, Status: x.Status,
		LatencyMS: x.Latency.Milliseconds(), Sent: x.Sent, FellBack: x.FellBack, Tier1: x.Tier1,
	}
	if x.Adapter != nil {
		e.Provider = x.Adapter.Name()
	}
	sum := sha256.Sum256(x.SentBody)
	e.SentSHA256 = hex.EncodeToString(sum[:])
	if x.ParseError != nil {
		e.ParseError = x.ParseError.Error()
	}
	if x.Original != nil {
		e.Model = x.Original.Model
	}
	if d := x.Decision; d != nil {
		e.Codec = d.Codec
		if x.Sent == "encoded" || x.FellBack {
			e.EstSavings = d.NetSavings
			if c, ok := codec.Get(d.Codec); ok {
				e.CodecVersion = c.Version()
			}
		}
		for _, b := range d.Blocks {
			g := BlockGate{Transform: b.Transform, JSONTokens: b.JSONTokens, EncTokens: b.EncTokens}
			if b.FailedGate != policy.GateNone {
				g.FailedGate, g.Reason = b.FailedGate.String(), b.Reason
			}
			e.Gates = append(e.Gates, g)
		}
	}
	if x.Response != nil {
		u := x.Response.Usage
		e.Usage = &u
	}
	if pol.EnableRawPayloadLogging && x.Original != nil {
		e.RequestRedacted = NewRedactor(pol.RedactKeys).RedactJSON(x.Original.Raw)
	}
	return e
}
