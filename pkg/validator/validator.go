// Package validator implements Tier 1 synchronous validation:
// Schema → Rules → Authorization over every tool call and structured
// output in a response. It is default-deny and all-or-nothing.
package validator

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	"github.com/santhosh-tekuri/jsonschema/v6"

	"github.com/AkashAgarwalInd/trimproof/pkg/ir"
)

// SecurityContext is the caller identity, taken only from trusted ingress,
// never from the request body.
type SecurityContext struct {
	Subject  string
	TenantID string
	Scopes   []string
	// ResourceConstraints restricts argument values: for each key, a tool
	// call argument with that name (at any depth) must hold one of the
	// listed values.
	ResourceConstraints map[string][]string
}

// HasScope reports whether s grants scope (or a "prefix:*" wildcard).
func (s SecurityContext) HasScope(scope string) bool {
	if slices.Contains(s.Scopes, scope) {
		return true
	}
	if i := strings.IndexByte(scope, ':'); i >= 0 {
		return slices.Contains(s.Scopes, scope[:i]+":*")
	}
	return false
}

// Step names the Tier 1 stage that produced a violation.
type Step string

const (
	StepContract Step = "contract" // default-deny structural checks
	StepSchema   Step = "schema"
	StepRule     Step = "rule"
	StepAuthz    Step = "authz"
)

// Violation is one reason a response was rejected.
type Violation struct {
	Step    Step   `json:"step"`
	Tool    string `json:"tool,omitempty"`
	CallID  string `json:"call_id,omitempty"`
	Rule    string `json:"rule,omitempty"`
	Message string `json:"message"`
}

// Rule is a business rule over one tool call. Rules must use exact
// arithmetic (big.Rat or integer minor units) for money.
type Rule interface {
	Name() string
	Check(call ir.ToolCall, args any, sec SecurityContext) []Violation
}

// Decision is an authorization outcome.
type Decision struct {
	Allow  bool
	Reason string
}

// Authorizer decides whether the caller may execute a tool call. OPA and
// Cerbos adapters implement the same interface.
type Authorizer interface {
	Authorize(ctx context.Context, call ir.ToolCall, args any, sec SecurityContext) (Decision, error)
}

// Config is the per-route Tier 1 configuration.
type Config struct {
	ToolSchemas  map[string][]byte // tool name -> JSON Schema
	OutputSchema []byte            // structured-output schema, if the route allows it
	Rules        []Rule
	Authorizer   Authorizer // nil means ScopeAuthorizer{}
}

// Validator is a compiled Config. It is safe for concurrent use.
type Validator struct {
	tools  map[string]*jsonschema.Schema
	output *jsonschema.Schema
	rules  []Rule
	authz  Authorizer
}

// New compiles all schemas up front so request-time validation does no
// compilation.
func New(cfg Config) (*Validator, error) {
	v := &Validator{tools: map[string]*jsonschema.Schema{}, rules: cfg.Rules, authz: cfg.Authorizer}
	if v.authz == nil {
		v.authz = ScopeAuthorizer{}
	}
	for name, raw := range cfg.ToolSchemas {
		s, err := compile("tool/"+name, raw)
		if err != nil {
			return nil, fmt.Errorf("validator: tool %q schema: %w", name, err)
		}
		v.tools[name] = s
	}
	if len(cfg.OutputSchema) > 0 {
		s, err := compile("output", cfg.OutputSchema)
		if err != nil {
			return nil, fmt.Errorf("validator: output schema: %w", err)
		}
		v.output = s
	}
	return v, nil
}

func compile(name string, raw []byte) (*jsonschema.Schema, error) {
	doc, err := jsonschema.UnmarshalJSON(bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}
	c := jsonschema.NewCompiler()
	url := "mem://trimproof/" + name + ".json"
	if err := c.AddResource(url, doc); err != nil {
		return nil, err
	}
	return c.Compile(url)
}

// Result is the Tier 1 outcome for a response.
type Result struct {
	OK         bool        `json:"ok"`
	Violations []Violation `json:"violations,omitempty"`
}

// Validate checks every tool call and the structured output. Every call
// goes through all three steps independently; any violation fails the
// whole response. No check is skipped because a field is absent.
func (v *Validator) Validate(ctx context.Context, req *ir.Request, resp *ir.Response, sec SecurityContext) Result {
	var vs []Violation
	declared := map[string]bool{}
	for _, t := range req.Tools {
		declared[t.Name] = true
	}
	for _, call := range resp.ToolCalls {
		vs = append(vs, v.checkCall(ctx, call, declared, sec)...)
	}

	switch {
	case resp.Structured != nil && v.output == nil:
		vs = append(vs, Violation{Step: StepContract, Message: "structured output on a route with no output schema"})
	case resp.Structured != nil:
		doc, err := jsonschema.UnmarshalJSON(bytes.NewReader(resp.Structured))
		if err != nil {
			vs = append(vs, Violation{Step: StepSchema, Message: "structured output is not valid JSON: " + err.Error()})
		} else if err := v.output.Validate(doc); err != nil {
			vs = append(vs, Violation{Step: StepSchema, Message: flatten(err)})
		}
	case req.StructuredOutput && len(resp.ToolCalls) == 0:
		vs = append(vs, Violation{Step: StepContract, Message: "structured output was requested but the response has none"})
	}
	return Result{OK: len(vs) == 0, Violations: vs}
}

func (v *Validator) checkCall(ctx context.Context, call ir.ToolCall, declared map[string]bool, sec SecurityContext) []Violation {
	viol := func(step Step, msg string) Violation {
		return Violation{Step: step, Tool: call.Name, CallID: call.ID, Message: msg}
	}
	if !declared[call.Name] {
		return []Violation{viol(StepContract, "tool was not declared in the request")}
	}
	schema, ok := v.tools[call.Name]
	if !ok {
		return []Violation{viol(StepContract, "tool has no schema on this route")}
	}
	args, err := jsonschema.UnmarshalJSON(bytes.NewReader(call.Args))
	if err != nil {
		return []Violation{viol(StepSchema, "arguments are not valid JSON: "+err.Error())}
	}
	if err := schema.Validate(args); err != nil {
		// Rules and authz assume schema-valid input; stop here for this call.
		return []Violation{viol(StepSchema, flatten(err))}
	}
	var vs []Violation
	for _, r := range v.rules {
		for _, rv := range r.Check(call, args, sec) {
			rv.Step, rv.Tool, rv.CallID, rv.Rule = StepRule, call.Name, call.ID, r.Name()
			vs = append(vs, rv)
		}
	}
	d, err := v.authz.Authorize(ctx, call, args, sec)
	switch {
	case err != nil:
		vs = append(vs, viol(StepAuthz, "authorizer error: "+err.Error()))
	case !d.Allow:
		vs = append(vs, viol(StepAuthz, d.Reason))
	}
	return vs
}

func flatten(err error) string {
	if ve, ok := err.(*jsonschema.ValidationError); ok {
		var parts []string
		var walk func(e *jsonschema.ValidationError)
		walk = func(e *jsonschema.ValidationError) {
			if len(e.Causes) == 0 {
				loc := "/" + strings.Join(e.InstanceLocation, "/")
				parts = append(parts, fmt.Sprintf("%s: %v", loc, e.ErrorKind))
			}
			for _, c := range e.Causes {
				walk(c)
			}
		}
		walk(ve)
		if len(parts) > 0 {
			return strings.Join(parts, "; ")
		}
	}
	return err.Error()
}

// ArgsJSON re-encodes parsed arguments, for messages and audit.
func ArgsJSON(args any) string {
	b, _ := json.Marshal(args)
	return string(b)
}
