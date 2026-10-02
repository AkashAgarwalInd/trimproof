package server

import (
	"bytes"
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

	_ "github.com/AkashAgarwalInd/trimproof/pkg/codec/tabular"
	_ "github.com/AkashAgarwalInd/trimproof/pkg/codec/toon"
	"github.com/AkashAgarwalInd/trimproof/pkg/policy"
)

var jwtKey = []byte("test-key")

// fakeUpstream records request bodies and replies from a queue.
type fakeUpstream struct {
	mu      sync.Mutex
	bodies  [][]byte
	headers []http.Header
	replies []reply
}

type reply struct {
	status int
	body   string
	sse    bool
}

func (f *fakeUpstream) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	b, _ := io.ReadAll(r.Body)
	f.mu.Lock()
	f.bodies = append(f.bodies, b)
	f.headers = append(f.headers, r.Header.Clone())
	rep := f.replies[0]
	if len(f.replies) > 1 {
		f.replies = f.replies[1:]
	}
	f.mu.Unlock()
	if rep.sse {
		w.Header().Set("Content-Type", "text/event-stream")
	} else {
		w.Header().Set("Content-Type", "application/json")
	}
	w.WriteHeader(rep.status)
	io.WriteString(w, rep.body)
}

func rowsJSON(n int) string {
	var b strings.Builder
	b.WriteString("[")
	for i := 0; i < n; i++ {
		if i > 0 {
			b.WriteString(",")
		}
		fmt.Fprintf(&b, `{"id":%d,"status":"refunded","amount":"%d.5","customer":"Customer %d"}`, i, i, i)
	}
	b.WriteString("]")
	return b.String()
}

func anthropicBody(toolResult string, stream bool) string {
	tr, _ := json.Marshal(toolResult)
	return fmt.Sprintf(`{"model":"claude-test","max_tokens":100,"stream":%v,
 "metadata":{"tenant_id":"evil-tenant"},
 "tools":[{"name":"refund","input_schema":{"type":"object"}}],
 "messages":[{"role":"user","content":"refund order 3"},
  {"role":"assistant","content":[{"type":"tool_use","id":"tu1","name":"lookup","input":{}}]},
  {"role":"user","content":[{"type":"tool_result","tool_use_id":"tu1","content":%s}]}]}`, stream, tr)
}

const refundSchema = `{"type":"object","required":["order_id","tenant_id"],"properties":{"order_id":{"type":"integer"},"tenant_id":{"type":"string"}}}`

func toolUseReply(tenant string) string {
	return fmt.Sprintf(`{"content":[{"type":"tool_use","id":"x1","name":"refund","input":{"order_id":3,"tenant_id":%q}}],"stop_reason":"tool_use","usage":{"input_tokens":50,"output_tokens":10}}`, tenant)
}

func setup(t *testing.T, state policy.PromotionState, validate bool, replies ...reply) (*httptest.Server, *fakeUpstream, *recorder) {
	t.Helper()
	up := &fakeUpstream{replies: replies}
	ups := httptest.NewServer(up)
	t.Cleanup(ups.Close)
	reg := NewRegistry()
	p := policy.Defaults()
	p.TenantID, p.RouteID, p.Codec, p.State = "t1", "support", "tabular", state
	p.MinPayloadTokens, p.MinNetSavings, p.AllowFallbackRetry = 50, 0.05, true
	p.ToolSchemas = map[string][]byte{"refund": []byte(refundSchema)}
	if err := reg.Put(p, nil, validate); err != nil {
		t.Fatal(err)
	}
	rec := &recorder{}
	gw := httptest.NewServer(New(Config{Registry: reg, AnthropicBase: ups.URL, OpenAIBase: ups.URL + "/v1",
		IdentityMode: IdentityJWT, IdentityKey: jwtKey, Observers: []Observer{rec}}))
	t.Cleanup(gw.Close)
	return gw, up, rec
}

type recorder struct {
	mu sync.Mutex
	xs []*Exchange
}

func (r *recorder) Observe(_ context.Context, x *Exchange) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.xs = append(r.xs, x)
}

func call(t *testing.T, gw *httptest.Server, path, body string, scopes ...string) (*http.Response, string) {
	t.Helper()
	req, _ := http.NewRequest("POST", gw.URL+path, strings.NewReader(body))
	req.Header.Set("x-api-key", "client-key")
	req.Header.Set(HeaderRoute, "support")
	req.Header.Set(HeaderIdentity, SignHS256(map[string]any{
		"sub": "agent-1", "tenant_id": "t1", "scope": strings.Join(scopes, " "), "exp": time.Now().Add(time.Hour).Unix(),
	}, jwtKey))
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	return resp, string(b)
}

func TestEligibleToolResultIsTransformed(t *testing.T) {
	gw, up, _ := setup(t, policy.Enabled, false, reply{200, `{"content":[{"type":"text","text":"ok"}],"usage":{"input_tokens":5,"output_tokens":1}}`, false})
	resp, _ := call(t, gw, "/anthropic/v1/messages", anthropicBody(rowsJSON(40), false))
	if resp.StatusCode != 200 {
		t.Fatalf("status %d", resp.StatusCode)
	}
	sent := string(up.bodies[0])
	if !strings.Contains(sent, `[\"amount\",\"customer\",\"id\",\"status\"]`) {
		t.Fatalf("tool result not tabular-encoded: %s", sent)
	}
	if !strings.Contains(sent, "table's first line") {
		t.Fatal("primer missing")
	}
	if !strings.HasPrefix(resp.Header.Get("X-Trimproof-Representation"), "tabular") {
		t.Fatalf("representation header %q", resp.Header.Get("X-Trimproof-Representation"))
	}
	h := up.headers[0]
	if h.Get(HeaderIdentity) != "" || h.Get(HeaderRoute) != "" || h.Get("X-Api-Key") != "client-key" {
		t.Fatalf("header hygiene: %v", h)
	}
}

func TestIneligibleIsByteIdentical(t *testing.T) {
	for _, state := range []policy.PromotionState{policy.Off, policy.Shadow, policy.Enabled} {
		gw, up, _ := setup(t, state, false, reply{200, `{"content":[]}`, false})
		for _, body := range []string{
			anthropicBody(rowsJSON(2), false),  // too small
			anthropicBody("plain text", false), // not JSON
			`{"model":"m","messages":[{"role":"user","content":"hi <b>&</b>"}]}`,
			`{not even json`,
		} {
			up.bodies = nil
			call(t, gw, "/anthropic/v1/messages", body)
			if !bytes.Equal(up.bodies[0], []byte(body)) {
				t.Fatalf("state %v: body changed:\n%s\n->\n%s", state, body, up.bodies[0])
			}
		}
	}
	// SHADOW never changes production traffic, even when eligible.
	gw, up, _ := setup(t, policy.Shadow, false, reply{200, `{"content":[]}`, false})
	body := anthropicBody(rowsJSON(40), false)
	call(t, gw, "/anthropic/v1/messages", body)
	if string(up.bodies[0]) != body {
		t.Fatal("SHADOW route modified production request")
	}
}

func TestUnauthorizedToolCallBlocked(t *testing.T) {
	gw, _, _ := setup(t, policy.Off, true, reply{200, toolUseReply("t1"), false})
	resp, body := call(t, gw, "/anthropic/v1/messages", anthropicBody("x", false)) // no scopes
	if resp.StatusCode != 422 || !strings.Contains(body, "missing scope action:refund") {
		t.Fatalf("got %d %s", resp.StatusCode, body)
	}
	resp, _ = call(t, gw, "/anthropic/v1/messages", anthropicBody("x", false), "action:refund")
	if resp.StatusCode != 200 {
		t.Fatalf("authorized call got %d", resp.StatusCode)
	}
}

func TestIdentityInBodyIgnored(t *testing.T) {
	// Body metadata claims evil-tenant; the model proposes a call for it.
	// The JWT says t1, so the call is cross-tenant and must be blocked.
	gw, _, _ := setup(t, policy.Off, true, reply{200, toolUseReply("evil-tenant"), false})
	resp, body := call(t, gw, "/anthropic/v1/messages", anthropicBody("x", false), "action:refund")
	if resp.StatusCode != 422 || !strings.Contains(body, "tenant") {
		t.Fatalf("got %d %s", resp.StatusCode, body)
	}
}

func TestFallbackRetryOnTier1Failure(t *testing.T) {
	gw, up, rec := setup(t, policy.Enabled, true,
		reply{200, toolUseReply("evil-tenant"), false}, // encoded attempt fails Tier 1
		reply{200, toolUseReply("t1"), false})          // canonical JSON retry passes
	resp, _ := call(t, gw, "/anthropic/v1/messages", anthropicBody(rowsJSON(40), false), "action:refund")
	if resp.StatusCode != 200 || len(up.bodies) != 2 {
		t.Fatalf("status %d after %d upstream calls", resp.StatusCode, len(up.bodies))
	}
	if !strings.Contains(string(up.bodies[0]), `[\"amount\"`) || strings.Contains(string(up.bodies[1]), `[\"amount\"`) {
		t.Fatal("expected encoded first attempt and canonical JSON retry")
	}
	if x := rec.xs[0]; !x.FellBack || x.Sent != "original" || x.Tier1 == nil || !x.Tier1.OK {
		t.Fatalf("exchange %+v", x)
	}
	if resp.Header.Get("X-Trimproof-Representation") != "json (fallback)" {
		t.Fatalf("header %q", resp.Header.Get("X-Trimproof-Representation"))
	}
}

func TestProviderErrorNeverFallsBack(t *testing.T) {
	gw, up, _ := setup(t, policy.Enabled, true, reply{529, `{"type":"error","error":{"type":"overloaded_error"}}`, false})
	resp, body := call(t, gw, "/anthropic/v1/messages", anthropicBody(rowsJSON(40), false), "action:refund")
	if resp.StatusCode != 529 || len(up.bodies) != 1 || !strings.Contains(body, "overloaded") {
		t.Fatalf("got %d after %d calls: %s", resp.StatusCode, len(up.bodies), body)
	}
}

const sseBody = "event: message_start\ndata: {\"type\":\"message_start\",\"message\":{\"usage\":{\"input_tokens\":9}}}\n\n" +
	"event: content_block_start\ndata: {\"type\":\"content_block_start\",\"index\":0,\"content_block\":{\"type\":\"tool_use\",\"id\":\"x1\",\"name\":\"refund\"}}\n\n" +
	"event: content_block_delta\ndata: {\"type\":\"content_block_delta\",\"index\":0,\"delta\":{\"type\":\"input_json_delta\",\"partial_json\":\"{\\\"order_id\\\":3,\"}}\n\n" +
	"event: content_block_delta\ndata: {\"type\":\"content_block_delta\",\"index\":0,\"delta\":{\"type\":\"input_json_delta\",\"partial_json\":\"\\\"tenant_id\\\":\\\"TENANT\\\"}\"}}\n\n" +
	"event: message_delta\ndata: {\"type\":\"message_delta\",\"delta\":{\"stop_reason\":\"tool_use\"},\"usage\":{\"output_tokens\":7}}\n\n" +
	"event: message_stop\ndata: {\"type\":\"message_stop\"}\n\n"

func TestSSEPassThrough(t *testing.T) {
	sse := strings.ReplaceAll(sseBody, "TENANT", "t1")
	gw, _, _ := setup(t, policy.Off, false, reply{200, sse, true})
	resp, body := call(t, gw, "/anthropic/v1/messages", anthropicBody("x", true))
	if resp.StatusCode != 200 || body != sse || !strings.HasPrefix(resp.Header.Get("Content-Type"), "text/event-stream") {
		t.Fatalf("got %d %q", resp.StatusCode, resp.Header.Get("Content-Type"))
	}
}

func TestValidatedStreamIsBufferedAndChecked(t *testing.T) {
	gw, _, _ := setup(t, policy.Off, true, reply{200, strings.ReplaceAll(sseBody, "TENANT", "evil"), true})
	resp, body := call(t, gw, "/anthropic/v1/messages", anthropicBody("x", true), "action:refund")
	if resp.StatusCode != 422 || !strings.Contains(body, "tenant") {
		t.Fatalf("got %d %s", resp.StatusCode, body)
	}
	gw, _, _ = setup(t, policy.Off, true, reply{200, strings.ReplaceAll(sseBody, "TENANT", "t1"), true})
	resp, body = call(t, gw, "/anthropic/v1/messages", anthropicBody("x", true), "action:refund")
	if resp.StatusCode != 200 || !strings.Contains(body, "message_stop") {
		t.Fatalf("got %d %s", resp.StatusCode, body)
	}
}

func TestOpenAIRouteTransforms(t *testing.T) {
	gw, up, _ := setup(t, policy.Enabled, false, reply{200, `{"choices":[{"message":{"content":"ok"}}],"usage":{"prompt_tokens":5}}`, false})
	rows, _ := json.Marshal(rowsJSON(40))
	body := fmt.Sprintf(`{"model":"gpt-test","messages":[{"role":"user","content":"q"},
	 {"role":"assistant","content":null,"tool_calls":[{"id":"c1","type":"function","function":{"name":"lookup","arguments":"{}"}}]},
	 {"role":"tool","tool_call_id":"c1","content":%s}]}`, rows)
	resp, _ := call(t, gw, "/openai/v1/chat/completions", body)
	if resp.StatusCode != 200 || !strings.Contains(string(up.bodies[0]), `[\"amount\"`) {
		t.Fatalf("got %d, sent %s", resp.StatusCode, up.bodies[0])
	}
	var sent struct{ Messages []struct{ Role string } }
	json.Unmarshal(up.bodies[0], &sent)
	if sent.Messages[0].Role != "system" {
		t.Fatal("primer system message not inserted")
	}
}

func TestIdentityRequired(t *testing.T) {
	_, err := Identify(httptest.NewRequest("POST", "/", nil), IdentityJWT, jwtKey, time.Now())
	if err == nil {
		t.Fatal("missing token accepted")
	}
	r := httptest.NewRequest("POST", "/", nil)
	r.Header.Set(HeaderIdentity, SignHS256(map[string]any{"tenant_id": "t1", "exp": time.Now().Add(-time.Minute).Unix()}, jwtKey))
	if _, err := Identify(r, IdentityJWT, jwtKey, time.Now()); err == nil {
		t.Fatal("expired token accepted")
	}
	r.Header.Set(HeaderIdentity, SignHS256(map[string]any{"tenant_id": "t1", "exp": time.Now().Add(time.Hour).Unix()}, []byte("other")))
	if _, err := Identify(r, IdentityJWT, jwtKey, time.Now()); err == nil {
		t.Fatal("wrong-key token accepted")
	}
}
