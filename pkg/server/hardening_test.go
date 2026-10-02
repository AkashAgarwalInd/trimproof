package server

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/AkashAgarwalInd/trimproof/pkg/policy"
)

// gateway builds a gateway in front of an arbitrary upstream handler.
func gateway(t *testing.T, upstream http.Handler, mod func(*Config)) *httptest.Server {
	t.Helper()
	ups := httptest.NewServer(upstream)
	t.Cleanup(ups.Close)
	reg := NewRegistry()
	p := policy.Defaults()
	p.TenantID, p.RouteID, p.Codec, p.State = "t1", "support", "toon", policy.Enabled
	if err := reg.Put(p, nil, false); err != nil {
		t.Fatal(err)
	}
	cfg := Config{Registry: reg, AnthropicBase: ups.URL, OpenAIBase: ups.URL + "/v1",
		IdentityMode: IdentityJWT, IdentityKey: jwtKey}
	if mod != nil {
		mod(&cfg)
	}
	gw := httptest.NewServer(New(cfg))
	t.Cleanup(gw.Close)
	return gw
}

type panicEstimator struct{}

func (panicEstimator) Estimate(string, string) int { panic("estimator exploded") }

func TestPanicIsRecovered(t *testing.T) {
	ok := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { io.WriteString(w, `{"content":[]}`) })
	gw := gateway(t, ok, func(c *Config) { c.Estimator = panicEstimator{} })
	body := anthropicBody(rowsJSON(40), false)
	for i := 0; i < 2; i++ { // the second call proves the process kept serving
		resp, out := call(t, gw, "/anthropic/v1/messages", body)
		if resp.StatusCode != 500 || !strings.Contains(out, "internal_error") {
			t.Fatalf("call %d: got %d %s", i, resp.StatusCode, out)
		}
	}
	resp, err := http.Get(gw.URL + "/healthz")
	if err != nil || resp.StatusCode != 200 {
		t.Fatalf("healthz after panic: %v %v", resp, err)
	}
}

func TestRequestBodyLimit(t *testing.T) {
	ok := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { io.WriteString(w, `{}`) })
	gw := gateway(t, ok, func(c *Config) { c.MaxBodyBytes = 1000 })
	big := `{"model":"m","messages":[{"role":"user","content":"` + strings.Repeat("x", 2000) + `"}]}`
	// Both the intercepted endpoint and the pass-through proxy enforce it;
	// the proxy used to truncate silently.
	for _, path := range []string{"/anthropic/v1/messages", "/anthropic/v1/messages/count_tokens"} {
		resp, out := call(t, gw, path, big)
		if resp.StatusCode != 413 || !strings.Contains(out, "request_too_large") {
			t.Fatalf("%s: got %d %s", path, resp.StatusCode, out)
		}
	}
	resp, _ := call(t, gw, "/anthropic/v1/messages", `{"model":"m","messages":[]}`)
	if resp.StatusCode != 200 {
		t.Fatalf("small body got %d", resp.StatusCode)
	}
}

func TestUpstreamTimeout(t *testing.T) {
	release := make(chan struct{})
	slow := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.ReadAll(r.Body)
		select {
		case <-release:
		case <-r.Context().Done():
		}
	})
	gw := gateway(t, slow, func(c *Config) { c.UpstreamTimeout = 100 * time.Millisecond })
	t.Cleanup(func() { close(release) }) // runs before the upstream closes
	for _, tc := range []struct{ path, body string }{
		{"/anthropic/v1/messages", anthropicBody("x", false)}, // buffered
		{"/anthropic/v1/messages", anthropicBody("x", true)},  // streamed: no headers in time
		{"/anthropic/v1/models", ""},                          // pass-through proxy
	} {
		start := time.Now()
		resp, out := call(t, gw, tc.path, tc.body)
		if resp.StatusCode != 504 || !strings.Contains(out, "upstream_timeout") {
			t.Fatalf("%s: got %d %s", tc.path, resp.StatusCode, out)
		}
		if d := time.Since(start); d > 2*time.Second {
			t.Fatalf("%s: timeout took %v", tc.path, d)
		}
	}
}

func TestStreamIdleTimeout(t *testing.T) {
	release := make(chan struct{})
	stall := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "text/event-stream")
		io.WriteString(w, "event: message_start\ndata: {}\n\n")
		w.(http.Flusher).Flush()
		select {
		case <-release:
		case <-r.Context().Done():
		}
	})
	gw := gateway(t, stall, func(c *Config) {
		c.UpstreamTimeout = time.Minute // a long generation is fine...
		c.StreamIdleTimeout = 150 * time.Millisecond
	})
	t.Cleanup(func() { close(release) })
	start := time.Now()
	resp, out := call(t, gw, "/anthropic/v1/messages", anthropicBody("x", true))
	if resp.StatusCode != 200 || !strings.Contains(out, "message_start") {
		t.Fatalf("got %d %q", resp.StatusCode, out)
	}
	if d := time.Since(start); d > 2*time.Second { // ...a silent one is not
		t.Fatalf("stalled stream held for %v", d)
	}
}

func TestStreamOutlivesUpstreamTimeout(t *testing.T) {
	// A stream that keeps producing chunks is not cut by UpstreamTimeout.
	steady := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		for i := 0; i < 6; i++ {
			io.WriteString(w, "data: {}\n\n")
			w.(http.Flusher).Flush()
			time.Sleep(50 * time.Millisecond)
		}
		io.WriteString(w, "data: [DONE]\n\n")
	})
	gw := gateway(t, steady, func(c *Config) {
		c.UpstreamTimeout = 100 * time.Millisecond
		c.StreamIdleTimeout = 200 * time.Millisecond
	})
	resp, out := call(t, gw, "/anthropic/v1/messages", anthropicBody("x", true))
	if resp.StatusCode != 200 || !strings.Contains(out, "[DONE]") {
		t.Fatalf("got %d %q", resp.StatusCode, out)
	}
}

func TestUpstreamResponseLimit(t *testing.T) {
	huge := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		io.WriteString(w, `{"content":[{"type":"text","text":"`+strings.Repeat("y", 5000)+`"}]}`)
	})
	gw := gateway(t, huge, func(c *Config) { c.MaxResponseBytes = 1000 })
	resp, out := call(t, gw, "/anthropic/v1/messages", anthropicBody("x", false))
	if resp.StatusCode != 502 || !strings.Contains(out, "size limit") {
		t.Fatalf("got %d %s", resp.StatusCode, out)
	}
}
