package provider_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/context-mesh/context-mesh/pkg/ir"
	"github.com/context-mesh/context-mesh/pkg/provider"
	"github.com/context-mesh/context-mesh/pkg/provider/anthropic"
	"github.com/context-mesh/context-mesh/pkg/provider/openai"
)

const anthropicReq = `{"model":"claude-x","max_tokens":100,"metadata":{"user_id":"u1"},
 "system":[{"type":"text","text":"Be terse.","cache_control":{"type":"ephemeral"}}],
 "tools":[{"name":"query_orders","input_schema":{"type":"object"}}],
 "messages":[
  {"role":"user","content":"How many refunded?"},
  {"role":"assistant","content":[{"type":"tool_use","id":"toolu_1","name":"query_orders","input":{"limit":2}}]},
  {"role":"user","content":[
    {"type":"tool_result","tool_use_id":"toolu_1","content":"[{\"id\":1,\"status\":\"paid\"},{\"id\":2,\"status\":\"refunded\"}]"},
    {"type":"text","text":"thanks"}]}]}`

const openaiReq = `{"model":"gpt-x","temperature":0,"seed":7,
 "messages":[
  {"role":"system","content":"Be terse."},
  {"role":"user","content":"How many refunded?"},
  {"role":"assistant","content":null,"tool_calls":[{"id":"call_1","type":"function","function":{"name":"query_orders","arguments":"{\"limit\":2}"}}]},
  {"role":"tool","tool_call_id":"call_1","content":"[{\"id\":1,\"status\":\"paid\"},{\"id\":2,\"status\":\"refunded\"}]"}],
 "tools":[{"type":"function","function":{"name":"query_orders","parameters":{"type":"object"}}}]}`

func adapters() map[string]struct {
	a   provider.Adapter
	req string
} {
	return map[string]struct {
		a   provider.Adapter
		req string
	}{
		"anthropic": {anthropic.Adapter{}, anthropicReq},
		"openai":    {openai.Adapter{}, openaiReq},
	}
}

func TestParseFindsToolResult(t *testing.T) {
	for name, c := range adapters() {
		req, err := c.a.ParseRequest([]byte(c.req))
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		db := req.DataBlocks()
		if len(db) != 1 {
			t.Fatalf("%s: %d data blocks", name, len(db))
		}
		if db[0].ToolName != "query_orders" || db[0].Kind != ir.ToolResult {
			t.Fatalf("%s: block %+v", name, db[0])
		}
		if len(req.Tools) != 1 || req.Tools[0].Name != "query_orders" {
			t.Fatalf("%s: tools %+v", name, req.Tools)
		}
	}
}

func TestPassThroughIsByteIdentical(t *testing.T) {
	for name, c := range adapters() {
		req, _ := c.a.ParseRequest([]byte(c.req))
		out, err := c.a.RenderRequest(req)
		if err != nil {
			t.Fatal(err)
		}
		if string(out) != c.req {
			t.Fatalf("%s: pass-through changed the body", name)
		}
	}
}

func TestRenderPatchesOnlyDataAndPrimer(t *testing.T) {
	for name, c := range adapters() {
		req, _ := c.a.ParseRequest([]byte(c.req))
		req.DataBlocks()[0].Transform = &ir.AppliedTransform{Codec: "tabular", Encoded: []byte("[\"id\",\"status\"]\n[1,\"paid\"]\n[2,\"refunded\"]")}
		req.Primer = "PRIMER <text>"
		out, err := c.a.RenderRequest(req)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		s := string(out)
		if !strings.Contains(s, `[\"id\",\"status\"]\n[1,\"paid\"]`) {
			t.Errorf("%s: encoded payload missing: %s", name, s)
		}
		if strings.Contains(s, `{\"id\":1`) {
			t.Errorf("%s: original payload still present", name)
		}
		if !strings.Contains(s, "PRIMER <text>") {
			t.Errorf("%s: primer missing or HTML-escaped: %s", name, s)
		}
		// Unmodelled fields survive.
		for _, keep := range []string{`"user_id":"u1"`, `"cache_control"`, `"seed":7`, `"thanks"`} {
			if strings.Contains(c.req, keep) && !strings.Contains(s, keep) {
				t.Errorf("%s: lost %s", name, keep)
			}
		}
		// Re-parse: the transformed body is still a valid request whose
		// tool result now holds the encoded text.
		again, err := c.a.ParseRequest(out)
		if err != nil {
			t.Fatalf("%s: reparse: %v", name, err)
		}
		var found bool
		for _, m := range again.Messages {
			for _, b := range m.Blocks {
				if b.Kind == ir.ToolResult && strings.HasPrefix(b.Text, `["id","status"]`) {
					found = true
				}
			}
		}
		if !found {
			t.Errorf("%s: reparsed body lacks encoded tool result", name)
		}
	}
}

func TestPrimerPlacement(t *testing.T) {
	a := anthropic.Adapter{}
	for _, body := range []string{
		`{"model":"m","messages":[]}`,
		`{"model":"m","system":"S","messages":[]}`,
		`{"model":"m","system":[{"type":"text","text":"S"}],"messages":[]}`,
	} {
		req, _ := a.ParseRequest([]byte(body))
		req.Primer = "P"
		out, err := a.RenderRequest(req)
		if err != nil {
			t.Fatal(err)
		}
		again, _ := a.ParseRequest(out)
		last := again.System[len(again.System)-1].Text
		if !strings.HasSuffix(last, "P") {
			t.Errorf("anthropic %s -> system %q", body, last)
		}
	}
	o := openai.Adapter{}
	req, _ := o.ParseRequest([]byte(`{"model":"m","messages":[{"role":"user","content":"hi"}]}`))
	req.Primer = "P"
	out, _ := o.RenderRequest(req)
	var w struct {
		Messages []struct{ Role, Content string }
	}
	_ = json.Unmarshal(out, &w)
	if w.Messages[0].Role != "system" || w.Messages[0].Content != "P" || w.Messages[1].Content != "hi" {
		t.Errorf("openai primer insert: %s", out)
	}
}

func TestErrorAndNonJSONResultsAreNotData(t *testing.T) {
	a := anthropic.Adapter{}
	req, _ := a.ParseRequest([]byte(`{"model":"m","messages":[
	 {"role":"assistant","content":[{"type":"tool_use","id":"t1","name":"f","input":{}},{"type":"tool_use","id":"t2","name":"g","input":{}}]},
	 {"role":"user","content":[
	  {"type":"tool_result","tool_use_id":"t1","is_error":true,"content":"[{\"a\":1}]"},
	  {"type":"tool_result","tool_use_id":"t2","content":"plain text"}]}]}`))
	if n := len(req.DataBlocks()); n != 0 {
		t.Fatalf("got %d data blocks", n)
	}
}

func TestParseResponses(t *testing.T) {
	ar, err := anthropic.Adapter{}.ParseResponse(&ir.Request{}, []byte(`{"content":[
	  {"type":"text","text":"ok"},
	  {"type":"tool_use","id":"tu1","name":"refund","input":{"order_id":5}},
	  {"type":"tool_use","id":"tu2","name":"notify","input":{}}],
	 "stop_reason":"tool_use","usage":{"input_tokens":10,"cache_read_input_tokens":90,"output_tokens":5}}`))
	if err != nil {
		t.Fatal(err)
	}
	if len(ar.ToolCalls) != 2 || ar.ToolCalls[0].Name != "refund" || string(ar.ToolCalls[0].Args) != `{"order_id":5}` {
		t.Fatalf("anthropic tool calls %+v", ar.ToolCalls)
	}
	if ar.Usage.InputTokens != 100 || ar.Usage.CachedInputTokens != 90 {
		t.Fatalf("anthropic usage %+v", ar.Usage)
	}

	req := &ir.Request{StructuredOutput: true}
	or, err := openai.Adapter{}.ParseResponse(req, []byte(`{"choices":[{"finish_reason":"tool_calls","message":{"content":"{\"x\":1}",
	  "tool_calls":[{"id":"c1","type":"function","function":{"name":"refund","arguments":"{\"order_id\":5}"}}]}}],
	 "usage":{"prompt_tokens":42,"completion_tokens":7,"prompt_tokens_details":{"cached_tokens":32}}}`))
	if err != nil {
		t.Fatal(err)
	}
	if len(or.ToolCalls) != 1 || string(or.ToolCalls[0].Args) != `{"order_id":5}` || string(or.Structured) != `{"x":1}` {
		t.Fatalf("openai %+v", or)
	}
	if or.Usage.InputTokens != 42 || or.Usage.CachedInputTokens != 32 {
		t.Fatalf("openai usage %+v", or.Usage)
	}
}
