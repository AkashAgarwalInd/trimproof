package eval

import (
	"bytes"
	"context"
	crand "crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"math/rand/v2"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"golang.org/x/sync/errgroup"
	"golang.org/x/time/rate"

	"github.com/AkashAgarwalInd/trimproof/pkg/codec"
	"github.com/AkashAgarwalInd/trimproof/pkg/ir"
	"github.com/AkashAgarwalInd/trimproof/pkg/policy"
	"github.com/AkashAgarwalInd/trimproof/pkg/provider"
	"github.com/AkashAgarwalInd/trimproof/pkg/server"
)

// Config configures the shadow evaluator.
type Config struct {
	Store      Store
	Comparator Comparator
	HTTP       *http.Client
	// ControlFraction of sampled requests become control pairs.
	ControlFraction float64
	QueueSize       int
	Workers         int
	// Per-provider budget for evaluation traffic, separate from production:
	// requests per second, burst, and max in-flight requests.
	RPS         float64
	Burst       int
	MaxInFlight int
	Promoter    *Promoter // optional; re-evaluated after every pair
	Log         *slog.Logger
	Rand        func() float64
}

// Evaluator samples served exchanges and runs paired arms off the latency
// path. It never affects the response the caller received.
type Evaluator struct {
	cfg     Config
	queue   chan *server.Exchange
	dropped atomic.Int64

	mu      sync.Mutex
	budgets map[string]*budget
}

type budget struct {
	lim *rate.Limiter
	sem chan struct{}
}

// NewEvaluator applies defaults.
func NewEvaluator(cfg Config) *Evaluator {
	if cfg.Comparator == nil {
		cfg.Comparator = DefaultComparator{}
	}
	if cfg.HTTP == nil {
		cfg.HTTP = &http.Client{Timeout: 10 * time.Minute}
	}
	if cfg.ControlFraction <= 0 {
		cfg.ControlFraction = 0.25
	}
	if cfg.QueueSize <= 0 {
		cfg.QueueSize = 256
	}
	if cfg.Workers <= 0 {
		cfg.Workers = 4
	}
	if cfg.RPS <= 0 {
		cfg.RPS = 1
	}
	if cfg.Burst <= 0 {
		cfg.Burst = 2
	}
	if cfg.MaxInFlight <= 0 {
		cfg.MaxInFlight = 4
	}
	if cfg.Log == nil {
		cfg.Log = slog.Default()
	}
	if cfg.Rand == nil {
		cfg.Rand = rand.Float64
	}
	return &Evaluator{cfg: cfg, queue: make(chan *server.Exchange, cfg.QueueSize), budgets: map[string]*budget{}}
}

// Dropped reports samples discarded because the queue was full.
func (e *Evaluator) Dropped() int64 { return e.dropped.Load() }

// Observe implements server.Observer. It samples and enqueues; it never
// blocks the request goroutine.
func (e *Evaluator) Observe(_ context.Context, x *server.Exchange) {
	if x.Route == nil || x.Original == nil || x.Decision == nil || x.Status != http.StatusOK {
		return
	}
	st := x.Route.Policy.State
	if st != policy.Shadow && st != policy.Enabled {
		return
	}
	if !x.Decision.Any() || e.cfg.Rand() >= x.Route.Policy.ShadowSampleRate {
		return
	}
	select {
	case e.queue <- x:
	default:
		e.dropped.Add(1)
	}
}

// Run starts workers and blocks until ctx is done and in-flight pairs have
// finished. Pairs cut short by ctx are discarded, not stored.
func (e *Evaluator) Run(ctx context.Context) {
	var wg sync.WaitGroup
	for i := 0; i < e.cfg.Workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-ctx.Done():
					return
				case x := <-e.queue:
					e.process(ctx, x)
				}
			}
		}()
	}
	wg.Wait()
}

// Drain processes everything currently queued (tests and shutdown).
func (e *Evaluator) Drain(ctx context.Context) {
	for {
		select {
		case x := <-e.queue:
			e.process(ctx, x)
		default:
			return
		}
	}
}

func (e *Evaluator) process(ctx context.Context, x *server.Exchange) {
	p, err := e.RunPair(ctx, x, e.cfg.Rand() < e.cfg.ControlFraction)
	if ctx.Err() != nil {
		return // shutting down: an aborted arm says nothing about the codec
	}
	if err != nil {
		e.cfg.Log.Warn("shadow pair failed", "err", err)
		return
	}
	if err := e.cfg.Store.Add(*p); err != nil {
		e.cfg.Log.Error("store pair", "err", err)
	}
	if e.cfg.Promoter != nil {
		if tr := e.cfg.Promoter.Evaluate(p.TenantID, p.RouteID); tr != nil {
			e.cfg.Log.Info("route state changed", "tenant", tr.TenantID, "route", tr.RouteID,
				"from", tr.From.String(), "to", tr.To.String(), "reason", tr.Reason)
		}
	}
}

// RunPair executes one treatment (or control) pair for exchange x. Both
// arms run concurrently with pinned, identical parameters and stream
// disabled. The gateway never executes tools, so arms are side-effect free.
func (e *Evaluator) RunPair(ctx context.Context, x *server.Exchange, control bool) (*EvaluationPair, error) {
	pol := x.Route.Policy
	c, ok := codec.Get(x.Decision.Codec)
	if !ok {
		return nil, fmt.Errorf("unknown codec %q", x.Decision.Codec)
	}
	bodyA, err := nonStreaming(x.Original.Raw)
	if err != nil {
		return nil, err
	}
	bodyB := bodyA
	kind := Control
	if !control {
		kind = Treatment
		enc, err := server.RenderWithDecision(x.Adapter, x.Original, x.Decision)
		if err != nil {
			return nil, err
		}
		if bodyB, err = nonStreaming(enc); err != nil {
			return nil, err
		}
	}
	nonStreamReq := *x.Original
	nonStreamReq.Stream = false

	pair := &EvaluationPair{ID: newID(), Time: time.Now(), Kind: kind, TenantID: pol.TenantID, RouteID: pol.RouteID,
		Model: x.Original.Model, Codec: c.Name(), CodecVersion: c.Version()}
	var respA, respB *ir.Response
	g, gctx := errgroup.WithContext(ctx)
	g.Go(func() error {
		var err error
		respA, err = e.arm(gctx, x, bodyA, &nonStreamReq)
		if err != nil {
			pair.ErrA = err.Error()
		}
		return nil
	})
	g.Go(func() error {
		var err error
		respB, err = e.arm(gctx, x, bodyB, &nonStreamReq)
		if err != nil {
			pair.ErrB = err.Error()
		}
		return nil
	})
	_ = g.Wait()
	if respA != nil {
		pair.UsageA = respA.Usage
	}
	if respB != nil {
		pair.UsageB = respB.Usage
	}
	if respA == nil || respB == nil {
		return pair, nil
	}
	pair.Agreement = e.cfg.Comparator.Compare(respA, respB)
	if v := x.Route.Validator; v != nil {
		a := v.Validate(ctx, x.Original, respA, x.Sec).OK
		b := v.Validate(ctx, x.Original, respB, x.Sec).OK
		pair.Tier1A, pair.Tier1B = &a, &b
	}
	return pair, nil
}

func (e *Evaluator) arm(ctx context.Context, x *server.Exchange, body []byte, req *ir.Request) (*ir.Response, error) {
	b := e.budget(x.Adapter.Name())
	if err := b.lim.Wait(ctx); err != nil {
		return nil, err
	}
	select {
	case b.sem <- struct{}{}:
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	defer func() { <-b.sem }()
	hr, err := http.NewRequestWithContext(ctx, http.MethodPost, x.Upstream, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	hr.Header = x.Header.Clone()
	resp, err := e.cfg.HTTP.Do(hr)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("upstream HTTP %d", resp.StatusCode)
	}
	return x.Adapter.ParseResponse(req, data)
}

func (e *Evaluator) budget(prov string) *budget {
	e.mu.Lock()
	defer e.mu.Unlock()
	b := e.budgets[prov]
	if b == nil {
		b = &budget{lim: rate.NewLimiter(rate.Limit(e.cfg.RPS), e.cfg.Burst), sem: make(chan struct{}, e.cfg.MaxInFlight)}
		e.budgets[prov] = b
	}
	return b
}

// nonStreaming sets "stream": false on a request body.
func nonStreaming(body []byte) ([]byte, error) {
	var top map[string]json.RawMessage
	if err := json.Unmarshal(body, &top); err != nil {
		return nil, err
	}
	if s, ok := top["stream"]; !ok || string(s) == "false" {
		return body, nil
	}
	top["stream"] = json.RawMessage("false")
	delete(top, "stream_options")
	return provider.Marshal(top)
}

func newID() string {
	var b [8]byte
	_, _ = crand.Read(b[:])
	return hex.EncodeToString(b[:])
}
