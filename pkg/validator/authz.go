package validator

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"

	"github.com/AkashAgarwalInd/context-mesh/pkg/ir"
)

// ScopeAuthorizer is the built-in authorizer (spec §7.3):
//   - the caller needs scope "action:<tool>" (or "action:*");
//   - every tenant-bearing argument, at any depth, must equal the caller's
//     TenantID;
//   - every argument named in ResourceConstraints must hold an allowed value.
type ScopeAuthorizer struct {
	// TenantKeys lists argument names that carry a tenant. Default:
	// tenant_id, tenantId, tenant.
	TenantKeys []string
}

var defaultTenantKeys = []string{"tenant_id", "tenantId", "tenant"}

func (a ScopeAuthorizer) Authorize(_ context.Context, call ir.ToolCall, args any, sec SecurityContext) (Decision, error) {
	if !sec.HasScope("action:" + call.Name) {
		return Decision{Reason: fmt.Sprintf("missing scope action:%s", call.Name)}, nil
	}
	keys := a.TenantKeys
	if keys == nil {
		keys = defaultTenantKeys
	}
	var deny string
	walk(args, func(key string, val any) bool {
		if slices.Contains(keys, key) {
			if sec.TenantID == "" || scalarString(val) != sec.TenantID {
				deny = fmt.Sprintf("argument %q does not match the caller's tenant", key)
				return false
			}
		}
		if allowed, ok := sec.ResourceConstraints[key]; ok && !slices.Contains(allowed, scalarString(val)) {
			deny = fmt.Sprintf("argument %q=%s is outside the caller's resource constraints", key, scalarString(val))
			return false
		}
		return true
	})
	if deny != "" {
		return Decision{Reason: deny}, nil
	}
	return Decision{Allow: true}, nil
}

// walk visits every object member at any depth; f returns false to stop.
func walk(v any, f func(key string, val any) bool) bool {
	switch t := v.(type) {
	case map[string]any:
		for k, val := range t {
			if !f(k, val) || !walk(val, f) {
				return false
			}
		}
	case []any:
		for _, e := range t {
			if !walk(e, f) {
				return false
			}
		}
	}
	return true
}

// scalarString renders a scalar for comparison. Containers never match an
// allowed value, so a tenant id smuggled inside an object is denied.
func scalarString(v any) string {
	switch t := v.(type) {
	case string:
		return t
	case json.Number:
		return t.String()
	case bool:
		return fmt.Sprint(t)
	case nil:
		return "null"
	}
	return "\x00container"
}
