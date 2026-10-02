package audit

import (
	"context"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"

	"github.com/AkashAgarwalInd/trimproof/pkg/eval"
	"github.com/AkashAgarwalInd/trimproof/pkg/policy"
	"github.com/AkashAgarwalInd/trimproof/pkg/server"
)

// Metrics records OpenTelemetry metrics for every exchange (spec §9):
// requests, gate rejections, estimated savings, Tier 1 outcomes, fallbacks
// and promotion transitions. It uses the global MeterProvider, which is a
// no-op until the binary installs an exporter.
type Metrics struct {
	requests    metric.Int64Counter
	gateRejects metric.Int64Counter
	savedTokens metric.Int64Counter
	tier1       metric.Int64Counter
	fallbacks   metric.Int64Counter
	transitions metric.Int64Counter
	inputTokens metric.Int64Counter
	latency     metric.Float64Histogram
}

// NewMetrics creates instruments on the global meter.
func NewMetrics() (*Metrics, error) {
	m := otel.Meter("github.com/AkashAgarwalInd/trimproof")
	var err error
	var ms Metrics
	mk := func(name, desc string) metric.Int64Counter {
		if err != nil {
			return nil
		}
		var c metric.Int64Counter
		c, err = m.Int64Counter(name, metric.WithDescription(desc))
		return c
	}
	ms.requests = mk("tp.requests", "Requests handled, by route, representation and status")
	ms.gateRejects = mk("tp.gate.rejections", "Data blocks rejected, by gate")
	ms.savedTokens = mk("tp.tokens.saved.estimated", "Estimated input tokens saved by encoding (net of primer)")
	ms.tier1 = mk("tp.tier1.results", "Tier 1 outcomes, by result and step")
	ms.fallbacks = mk("tp.fallbacks", "Canonical JSON retries after Tier 1 failure of an encoded request")
	ms.transitions = mk("tp.promotion.transitions", "Route promotion state transitions")
	ms.inputTokens = mk("tp.tokens.input", "Provider-reported input tokens, by representation")
	if err != nil {
		return nil, err
	}
	ms.latency, err = m.Float64Histogram("tp.request.duration", metric.WithUnit("s"))
	return &ms, err
}

// Observe implements server.Observer.
func (m *Metrics) Observe(ctx context.Context, x *server.Exchange) {
	if x.Route == nil {
		return
	}
	pol := x.Route.Policy
	route := attribute.String("route", pol.TenantID+"/"+pol.RouteID)
	sent := attribute.String("sent", x.Sent)
	m.requests.Add(ctx, 1, metric.WithAttributes(route, sent, attribute.Int("status", x.Status), attribute.String("state", pol.State.String())))
	m.latency.Record(ctx, x.Latency.Seconds(), metric.WithAttributes(route))
	if d := x.Decision; d != nil {
		for _, b := range d.Blocks {
			if b.FailedGate != policy.GateNone {
				m.gateRejects.Add(ctx, 1, metric.WithAttributes(route, attribute.String("gate", b.FailedGate.String())))
			}
		}
		if x.Sent == "encoded" {
			m.savedTokens.Add(ctx, int64(d.JSONTokens-d.EncTokens-d.PrimerTokens), metric.WithAttributes(route, attribute.String("codec", d.Codec)))
		}
	}
	if x.Response != nil {
		m.inputTokens.Add(ctx, int64(x.Response.Usage.InputTokens), metric.WithAttributes(route, sent))
	}
	if x.FellBack {
		m.fallbacks.Add(ctx, 1, metric.WithAttributes(route))
	}
	if x.Tier1 != nil {
		res := "pass"
		step := ""
		if !x.Tier1.OK {
			res = "fail"
			if len(x.Tier1.Violations) > 0 {
				step = string(x.Tier1.Violations[0].Step)
			}
		}
		m.tier1.Add(ctx, 1, metric.WithAttributes(route, attribute.String("result", res), attribute.String("step", step)))
	}
}

// Transition records a promotion state change.
func (m *Metrics) Transition(tr eval.Transition) {
	m.transitions.Add(context.Background(), 1, metric.WithAttributes(
		attribute.String("route", tr.TenantID+"/"+tr.RouteID),
		attribute.String("from", tr.From.String()), attribute.String("to", tr.To.String())))
}
