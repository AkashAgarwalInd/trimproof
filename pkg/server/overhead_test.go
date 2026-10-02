package server

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/AkashAgarwalInd/trimproof/pkg/policy"
)

// stubTransport answers every upstream call in-process, so a benchmark of
// the gateway measures only the gateway's own work.
type stubTransport struct{ body []byte }

func (s stubTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	io.Copy(io.Discard, r.Body)
	r.Body.Close()
	return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {"application/json"}},
		Body: io.NopCloser(bytes.NewReader(s.body)), Request: r}, nil
}

// payloadRows returns a JSON array of order-like rows of roughly size bytes.
func payloadRows(size int) string {
	var b strings.Builder
	b.WriteString("[")
	for i := 0; b.Len() < size; i++ {
		if i > 0 {
			b.WriteString(",")
		}
		fmt.Fprintf(&b, `{"order_id":%d,"customer":"Customer %d","status":%q,"amount":%v,"currency":"USD","created_at":"2026-09-%02dT10:%02d:00Z","items":%d}`,
			10000+i, i%97, []string{"paid", "refunded", "pending", "shipped"}[i%4], float64(1000+i*37%50000)/100, 1+i%28, i%60, 1+i%7)
	}
	b.WriteString("]")
	return b.String()
}

// BenchmarkGatewayOverhead measures the full gateway request path (parse,
// gates, encode, render, forward, respond) against an in-process upstream,
// for a request carrying one tool result of the given size. It reports
// p50/p99 per-request latency in addition to ns/op.
//
//	go test ./pkg/server -run '^$' -bench GatewayOverhead -benchtime 2000x
func BenchmarkGatewayOverhead(b *testing.B) {
	for _, size := range []int{10 << 10, 100 << 10} {
		body := []byte(anthropicBody(payloadRows(size), false))
		for _, st := range []policy.PromotionState{policy.Off, policy.Shadow, policy.Enabled} {
			b.Run(fmt.Sprintf("%dKB/%s", size>>10, st), func(b *testing.B) {
				reg := NewRegistry()
				p := policy.Defaults()
				p.TenantID, p.RouteID, p.Codec, p.State = "*", "support", "toon", st
				if err := reg.Put(p, nil, false); err != nil {
					b.Fatal(err)
				}
				s := New(Config{Registry: reg, AnthropicBase: "http://upstream.invalid",
					HTTP: &http.Client{Transport: stubTransport{body: []byte(`{"content":[{"type":"text","text":"ok"}],"usage":{"input_tokens":5,"output_tokens":1}}`)}}})
				lat := make([]time.Duration, 0, b.N)
				b.SetBytes(int64(len(body)))
				b.ReportAllocs()
				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					r := httptest.NewRequest("POST", "/anthropic/v1/messages", bytes.NewReader(body))
					r.Header.Set(HeaderRoute, "support")
					w := httptest.NewRecorder()
					t0 := time.Now()
					s.ServeHTTP(w, r)
					lat = append(lat, time.Since(t0))
					if w.Code != 200 {
						b.Fatalf("status %d: %s", w.Code, w.Body)
					}
					if st == policy.Enabled && !strings.HasPrefix(w.Header().Get("X-Trimproof-Representation"), "toon") {
						b.Fatalf("not encoded: %q", w.Header().Get("X-Trimproof-Representation"))
					}
				}
				b.StopTimer()
				slices.Sort(lat)
				q := func(f float64) float64 { return float64(lat[int(f*float64(len(lat)-1))].Microseconds()) / 1000 }
				b.ReportMetric(q(0.50), "p50-ms")
				b.ReportMetric(q(0.99), "p99-ms")
			})
		}
	}
}

// BenchmarkGatewayOverheadParallel is BenchmarkGatewayOverhead under
// concurrency: GOMAXPROCS×parallelism goroutines send requests at once, and
// it reports throughput as well as per-request p50/p99.
//
//	go test ./pkg/server -run '^$' -bench GatewayOverheadParallel -benchtime 2000x -cpu 8
func BenchmarkGatewayOverheadParallel(b *testing.B) {
	for _, size := range []int{10 << 10, 100 << 10} {
		body := []byte(anthropicBody(payloadRows(size), false))
		for _, st := range []policy.PromotionState{policy.Off, policy.Enabled} {
			b.Run(fmt.Sprintf("%dKB/%s", size>>10, st), func(b *testing.B) {
				reg := NewRegistry()
				p := policy.Defaults()
				p.TenantID, p.RouteID, p.Codec, p.State = "*", "support", "toon", st
				if err := reg.Put(p, nil, false); err != nil {
					b.Fatal(err)
				}
				s := New(Config{Registry: reg, AnthropicBase: "http://upstream.invalid",
					HTTP: &http.Client{Transport: stubTransport{body: []byte(`{"content":[{"type":"text","text":"ok"}],"usage":{"input_tokens":5,"output_tokens":1}}`)}}})
				var mu sync.Mutex
				lat := make([]time.Duration, 0, b.N)
				b.SetBytes(int64(len(body)))
				b.ReportAllocs()
				b.SetParallelism(4)
				start := time.Now()
				b.ResetTimer()
				b.RunParallel(func(pb *testing.PB) {
					var mine []time.Duration
					for pb.Next() {
						r := httptest.NewRequest("POST", "/anthropic/v1/messages", bytes.NewReader(body))
						r.Header.Set(HeaderRoute, "support")
						w := httptest.NewRecorder()
						t0 := time.Now()
						s.ServeHTTP(w, r)
						mine = append(mine, time.Since(t0))
						if w.Code != 200 {
							b.Errorf("status %d", w.Code)
							return
						}
					}
					mu.Lock()
					lat = append(lat, mine...)
					mu.Unlock()
				})
				b.StopTimer()
				elapsed := time.Since(start)
				slices.Sort(lat)
				q := func(f float64) float64 { return float64(lat[int(f*float64(len(lat)-1))].Microseconds()) / 1000 }
				b.ReportMetric(q(0.50), "p50-ms")
				b.ReportMetric(q(0.99), "p99-ms")
				b.ReportMetric(float64(len(lat))/elapsed.Seconds(), "req/s")
			})
		}
	}
}
