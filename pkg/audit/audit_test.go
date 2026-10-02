package audit

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"testing"
	"time"

	_ "github.com/AkashAgarwalInd/context-mesh/pkg/codec/tabular"
	"github.com/AkashAgarwalInd/context-mesh/pkg/ir"
	"github.com/AkashAgarwalInd/context-mesh/pkg/policy"
	"github.com/AkashAgarwalInd/context-mesh/pkg/provider/anthropic"
	"github.com/AkashAgarwalInd/context-mesh/pkg/server"
	"github.com/AkashAgarwalInd/context-mesh/pkg/validator"
)

func TestRedactByKeyAllTypes(t *testing.T) {
	r := NewRedactor([]string{"email"})
	in := `{"user":{"Email":"a@b.c","password":12345,"api-key":["x"],"ok":"keep"},
	 "rows":[{"ssn":null,"Card_Number":{"n":"4111"}}],
	 "tool_result":"[{\"token\":\"t-1\",\"id\":7}]","quote":"he said \"password\": x"}`
	out := string(r.RedactJSON([]byte(in)))
	for _, leak := range []string{"a@b.c", "12345", `"x"`, "4111", "t-1"} {
		if strings.Contains(out, leak) {
			t.Errorf("leaked %s in %s", leak, out)
		}
	}
	for _, keep := range []string{`"ok":"keep"`, `\"id\":7`, `he said \"password\": x`} {
		if !strings.Contains(out, keep) {
			t.Errorf("lost %s in %s", keep, out)
		}
	}
	if got := string(r.RedactJSON([]byte(`{bad json "password": 1`))); got != `"[REDACTED]"` {
		t.Errorf("unparseable input must fail closed, got %s", got)
	}
}

type memSink struct {
	mu sync.Mutex
	es []*Entry
}

func (m *memSink) Write(_ context.Context, e *Entry) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.es = append(m.es, e)
	return nil
}

func exchange(t *testing.T, sent string) *server.Exchange {
	body := []byte(`{"model":"m","messages":[{"role":"user","content":[{"type":"tool_result","tool_use_id":"a","content":"[{\"id\":1,\"password\":\"hunter2\"}]"}]}]}`)
	req, err := anthropic.Adapter{}.ParseRequest(body)
	if err != nil {
		t.Fatal(err)
	}
	p := policy.Defaults()
	p.TenantID, p.RouteID, p.Codec, p.AuditSampleRate, p.EnableRawPayloadLogging = "t1", "r", "tabular", 1, true
	return &server.Exchange{Start: time.Now(), Route: &server.Route{Policy: p}, Adapter: anthropic.Adapter{}, Original: req,
		Decision: &policy.Decision{Codec: "tabular", NetSavings: 0.3, Blocks: []policy.BlockDecision{{Transform: true}}},
		Sent:     sent, SentBody: []byte("[\"id\",\"password\"]\n[1,\"hunter2\"]"), Status: 200,
		Response: &ir.Response{Usage: ir.Usage{InputTokens: 10}}}
}

func TestEncodedPayloadNeverLoggedRaw(t *testing.T) {
	sink := &memSink{}
	a := New(Config{Sinks: []Sink{sink}})
	a.Observe(context.Background(), exchange(t, "encoded"))
	a.Close()
	if len(sink.es) != 1 {
		t.Fatalf("%d entries", len(sink.es))
	}
	b, _ := json.Marshal(sink.es[0])
	s := string(b)
	if strings.Contains(s, "hunter2") {
		t.Fatalf("secret leaked: %s", s)
	}
	e := sink.es[0]
	if e.Sent != "encoded" || e.Codec != "tabular" || e.CodecVersion != "1" || len(e.SentSHA256) != 64 {
		t.Fatalf("entry %+v", e)
	}
	if !strings.Contains(string(e.RequestRedacted), "[REDACTED]") {
		t.Fatalf("redacted original missing: %s", e.RequestRedacted)
	}
}

func TestSamplingAndAlwaysAudit(t *testing.T) {
	sink := &memSink{}
	a := New(Config{Sinks: []Sink{sink}, Rand: func() float64 { return 0.99 }})
	x := exchange(t, "original")
	x.Route.Policy.AuditSampleRate = 0.01
	a.Observe(context.Background(), x) // not sampled
	f := exchange(t, "original")
	f.Route.Policy.AuditSampleRate = 0.01
	f.Tier1 = &validator.Result{OK: false}
	a.Observe(context.Background(), f) // Tier 1 failure: always
	a.Close()
	if len(sink.es) != 1 || sink.es[0].Tier1 == nil {
		t.Fatalf("entries %+v", sink.es)
	}
}

type blockingSink struct{ ch chan struct{} }

func (b blockingSink) Write(context.Context, *Entry) error { <-b.ch; return nil }

func TestDropOnFull(t *testing.T) {
	bs := blockingSink{ch: make(chan struct{})}
	a := New(Config{Sinks: []Sink{bs}, QueueSize: 2, Workers: 1})
	start := time.Now()
	for i := 0; i < 50; i++ {
		a.Observe(context.Background(), exchange(t, "original"))
	}
	if time.Since(start) > time.Second {
		t.Fatal("Observe blocked")
	}
	if a.Dropped() < 45 {
		t.Fatalf("dropped %d", a.Dropped())
	}
	close(bs.ch)
	a.Close()
}
