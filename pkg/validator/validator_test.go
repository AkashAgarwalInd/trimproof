package validator

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/context-mesh/context-mesh/pkg/ir"
)

const refundSchema = `{"type":"object","required":["order_id","amount","tenant_id"],
 "properties":{"order_id":{"type":"integer"},"amount":{"type":"string","pattern":"^[0-9]+(\\.[0-9]{1,2})?$"},"tenant_id":{"type":"string"}},
 "additionalProperties":false}`

const notifySchema = `{"type":"object","required":["channel"],"properties":{"channel":{"type":"string"},"region":{"type":"string"}}}`

func newValidator(t *testing.T) *Validator {
	t.Helper()
	v, err := New(Config{
		ToolSchemas: map[string][]byte{"refund": []byte(refundSchema), "notify": []byte(notifySchema)},
		Rules:       []Rule{MaxAmount{RuleName: "refund-limit", Tool: "refund", Field: "amount", Max: "500.00"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	return v
}

var (
	req = &ir.Request{Tools: []ir.ToolDef{{Name: "refund"}, {Name: "notify"}, {Name: "undocumented"}}}
	sec = SecurityContext{TenantID: "t1", Scopes: []string{"action:refund", "action:notify"},
		ResourceConstraints: map[string][]string{"region": {"eu"}}}
)

func call(name, args string) ir.ToolCall {
	return ir.ToolCall{ID: "c-" + name, Name: name, Args: json.RawMessage(args)}
}

func TestTier1(t *testing.T) {
	v := newValidator(t)
	good := call("refund", `{"order_id":5,"amount":"499.99","tenant_id":"t1"}`)
	cases := []struct {
		name  string
		calls []ir.ToolCall
		sec   SecurityContext
		step  Step // expected first violation step; "" = OK
		msg   string
	}{
		{"valid", []ir.ToolCall{good}, sec, "", ""},
		{"unknown tool", []ir.ToolCall{call("delete_all", `{}`)}, sec, StepContract, "not declared"},
		{"declared but no schema", []ir.ToolCall{call("undocumented", `{}`)}, sec, StepContract, "no schema"},
		{"missing required field", []ir.ToolCall{call("refund", `{"order_id":5,"tenant_id":"t1"}`)}, sec, StepSchema, "amount"},
		{"extra field", []ir.ToolCall{call("refund", `{"order_id":5,"amount":"1","tenant_id":"t1","x":1}`)}, sec, StepSchema, ""},
		{"bad json args", []ir.ToolCall{call("refund", `{"order_id":`)}, sec, StepSchema, "not valid JSON"},
		{"rule: exact decimal over limit", []ir.ToolCall{call("refund", `{"order_id":5,"amount":"500.01","tenant_id":"t1"}`)}, sec, StepRule, "exceeds"},
		{"rule: at limit ok", []ir.ToolCall{call("refund", `{"order_id":5,"amount":"500.00","tenant_id":"t1"}`)}, sec, "", ""},
		{"authz: no scope", []ir.ToolCall{good}, SecurityContext{TenantID: "t1"}, StepAuthz, "missing scope"},
		{"authz: wildcard scope", []ir.ToolCall{good}, SecurityContext{TenantID: "t1", Scopes: []string{"action:*"}}, "", ""},
		{"authz: cross-tenant", []ir.ToolCall{call("refund", `{"order_id":5,"amount":"1","tenant_id":"t2"}`)}, sec, StepAuthz, "tenant"},
		{"authz: no caller tenant", []ir.ToolCall{good}, SecurityContext{Scopes: []string{"action:refund"}}, StepAuthz, "tenant"},
		{"authz: nested tenant", []ir.ToolCall{call("notify", `{"channel":"x","meta":{"tenant_id":"t2"}}`)}, sec, StepAuthz, "tenant"},
		{"authz: resource constraint", []ir.ToolCall{call("notify", `{"channel":"x","region":"us"}`)}, sec, StepAuthz, "resource"},
		// All-or-nothing: one bad call among parallel calls rejects the response.
		{"parallel: one unauthorized", []ir.ToolCall{good, call("notify", `{"channel":"x","region":"us"}`)}, sec, StepAuthz, "resource"},
		{"parallel: all good", []ir.ToolCall{good, call("notify", `{"channel":"x","region":"eu"}`)}, sec, "", ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			res := v.Validate(context.Background(), req, &ir.Response{ToolCalls: c.calls}, c.sec)
			if c.step == "" {
				if !res.OK {
					t.Fatalf("rejected: %+v", res.Violations)
				}
				return
			}
			if res.OK {
				t.Fatal("accepted")
			}
			if res.Violations[0].Step != c.step || !strings.Contains(res.Violations[0].Message, c.msg) {
				t.Fatalf("violation %+v, want step %s containing %q", res.Violations[0], c.step, c.msg)
			}
		})
	}
}

func TestStructuredOutput(t *testing.T) {
	noSchema := newValidator(t)
	sreq := &ir.Request{StructuredOutput: true}
	if res := noSchema.Validate(context.Background(), sreq, &ir.Response{Structured: json.RawMessage(`{"a":1}`)}, sec); res.OK {
		t.Fatal("structured output without route schema must be rejected")
	}
	v, err := New(Config{OutputSchema: []byte(`{"type":"object","required":["answer"],"properties":{"answer":{"type":"integer"}}}`)})
	if err != nil {
		t.Fatal(err)
	}
	cases := map[string]bool{`{"answer":3}`: true, `{"answer":"3"}`: false, `{}`: false, `not json`: false}
	for body, ok := range cases {
		if res := v.Validate(context.Background(), sreq, &ir.Response{Structured: json.RawMessage(body)}, sec); res.OK != ok {
			t.Errorf("%s: OK=%v (%+v)", body, res.OK, res.Violations)
		}
	}
	if res := v.Validate(context.Background(), sreq, &ir.Response{Text: ""}, sec); res.OK {
		t.Error("missing structured output must be rejected")
	}
}
