package calibrate

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	_ "github.com/AkashAgarwalInd/trimproof/pkg/codec/toon"
	"github.com/AkashAgarwalInd/trimproof/pkg/policy"
	"github.com/AkashAgarwalInd/trimproof/pkg/server"
	"github.com/AkashAgarwalInd/trimproof/pkg/tokens"
)

// fakeAnthropic answers /v1/messages, and /v1/messages/count_tokens with
// 7 overhead tokens plus one token per 3 bytes of the message text.
type fakeAnthropic struct {
	mu     sync.Mutex
	counts []string // texts counted
	keys   []string
}

func (f *fakeAnthropic) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	if r.URL.Path != "/v1/messages/count_tokens" {
		io.WriteString(w, `{"content":[{"type":"text","text":"ok"}],"usage":{"input_tokens":5,"output_tokens":1}}`)
		return
	}
	var req struct {
		Messages []struct{ Content string } `json:"messages"`
	}
	json.Unmarshal(body, &req)
	text := req.Messages[0].Content
	f.mu.Lock()
	f.counts = append(f.counts, text)
	f.keys = append(f.keys, r.Header.Get("X-Api-Key"))
	f.mu.Unlock()
	fmt.Fprintf(w, `{"input_tokens":%d}`, 7+(len(text)+2)/3)
}

func (f *fakeAnthropic) n() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.counts)
}

func rows(n int) string {
	var b strings.Builder
	b.WriteString("[")
	for i := 0; i < n; i++ {
		if i > 0 {
			b.WriteString(",")
		}
		fmt.Fprintf(&b, `{"id":%d,"status":"refunded","customer":"Customer %d"}`, i, i)
	}
	return b.String() + "]"
}

func TestCalibratesClaudeEstimates(t *testing.T) {
	up := &fakeAnthropic{}
	ups := httptest.NewServer(up)
	defer ups.Close()

	reg := server.NewRegistry()
	p := policy.Defaults()
	p.TenantID, p.RouteID, p.Codec, p.State = "*", "r", "toon", policy.Shadow
	if err := reg.Put(p, nil, false); err != nil {
		t.Fatal(err)
	}
	est := tokens.NewCalibrated(nil)
	cal := New(Config{Estimator: est, AnthropicBase: ups.URL, Interval: time.Hour})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go cal.Run(ctx)
	var (
		mu  sync.Mutex
		xs  []*server.Exchange
		rec = observerFunc(func(x *server.Exchange) { mu.Lock(); xs = append(xs, x); mu.Unlock() })
	)
	gw := httptest.NewServer(server.New(server.Config{Registry: reg, AnthropicBase: ups.URL, OpenAIBase: ups.URL,
		Estimator: est, Observers: []server.Observer{rec, cal}}))
	defer gw.Close()

	payload := rows(40)
	tr, _ := json.Marshal(payload)
	body := fmt.Sprintf(`{"model":"claude-test","max_tokens":10,"messages":[{"role":"user","content":"q"},
	 {"role":"assistant","content":[{"type":"tool_use","id":"t1","name":"lookup","input":{}}]},
	 {"role":"user","content":[{"type":"tool_result","tool_use_id":"t1","content":%s}]}]}`, tr)
	send := func(path, b string) {
		req, _ := http.NewRequest("POST", gw.URL+path, strings.NewReader(b))
		req.Header.Set("X-Trimproof-Route", "r")
		req.Header.Set("X-Api-Key", "client-key")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
	}

	send("/anthropic/v1/messages", body)
	deadline := time.Now().Add(5 * time.Second)
	for up.n() < 3 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if up.n() != 3 {
		t.Fatalf("%d count_tokens calls, want 3 (baseline, json, codec)", up.n())
	}
	for _, k := range up.keys {
		if k != "client-key" {
			t.Fatalf("count_tokens sent with key %q", k)
		}
	}
	mu.Lock()
	d := xs[0].Decision
	mu.Unlock()
	var canon, enc string
	for _, b := range d.Blocks {
		canon, enc = string(b.Canonical), string(b.Encoded)
	}
	if up.counts[1] != canon || up.counts[2] != enc {
		t.Fatal("count_tokens did not receive the block's JSON and encoded forms")
	}
	// Exact counts per the fake: (len+2)/3 after the 7-token overhead.
	wantJSON := float64((len(canon)+2)/3) / float64(est.Base.Count(canon))
	wantEnc := float64((len(enc)+2)/3) / float64(est.Base.Count(enc))
	if f := est.FactorKind("claude-test", tokens.KindJSON); f != wantJSON {
		t.Fatalf("json factor %v, want %v", f, wantJSON)
	}
	if f := est.FactorKind("claude-test", "toon"); f != wantEnc {
		t.Fatalf("toon factor %v, want %v", f, wantEnc)
	}
	if f := est.Factor("gpt-test"); f != 1 {
		t.Fatalf("other models affected: %v", f)
	}

	// The next request's gate estimates use the calibrated counts.
	send("/anthropic/v1/messages", body)
	mu.Lock()
	d = xs[1].Decision
	mu.Unlock()
	if d.JSONTokens != (len(canon)+2)/3 && d.JSONTokens != (len(canon)+2)/3+1 {
		t.Fatalf("calibrated JSON estimate %d, want ≈%d", d.JSONTokens, (len(canon)+2)/3)
	}
	// Within the interval no further calls are made, and OpenAI traffic
	// is never calibrated.
	send("/openai/v1/chat/completions", `{"model":"gpt-test","messages":[{"role":"user","content":"q"}]}`)
	time.Sleep(50 * time.Millisecond)
	if up.n() != 3 {
		t.Fatalf("%d count_tokens calls after the interval check", up.n())
	}
}

type observerFunc func(*server.Exchange)

func (f observerFunc) Observe(_ context.Context, x *server.Exchange) { f(x) }
