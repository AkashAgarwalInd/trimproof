package server

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/AkashAgarwalInd/context-mesh/pkg/validator"
)

// Identity headers. They are always stripped before forwarding upstream.
const (
	HeaderIdentity = "X-CM-Identity" // HS256 JWT minted by trusted ingress
	HeaderTenant   = "X-CM-Tenant"   // trusted-headers mode only
	HeaderSubject  = "X-CM-Subject"
	HeaderScopes   = "X-CM-Scopes" // space-separated
	HeaderRoute    = "X-Context-Mesh-Route"
	HeaderData     = "X-Context-Mesh-Data" // marked data blocks: "msg[:part],..."
)

// IdentityMode selects how the SecurityContext is established.
type IdentityMode string

const (
	// IdentityJWT verifies an HS256 JWT in X-CM-Identity. Recommended.
	IdentityJWT IdentityMode = "jwt-hs256"
	// IdentityTrustedHeaders reads X-CM-* headers as-is. Only safe when the
	// ingress in front of the gateway strips client-supplied copies.
	IdentityTrustedHeaders IdentityMode = "trusted-headers"
)

type jwtClaims struct {
	Sub      string              `json:"sub"`
	TenantID string              `json:"tenant_id"`
	Scope    any                 `json:"scope"` // "a b" or ["a","b"]
	RC       map[string][]string `json:"resource_constraints"`
	Exp      int64               `json:"exp"`
}

var errNoIdentity = errors.New("no identity")

// Identify builds the SecurityContext from trusted ingress data only
// (spec invariant 8). Identity fields inside the request body are never read.
func Identify(r *http.Request, mode IdentityMode, key []byte, now time.Time) (validator.SecurityContext, error) {
	switch mode {
	case IdentityTrustedHeaders:
		t := r.Header.Get(HeaderTenant)
		if t == "" {
			return validator.SecurityContext{}, errNoIdentity
		}
		return validator.SecurityContext{
			TenantID: t,
			Subject:  r.Header.Get(HeaderSubject),
			Scopes:   strings.Fields(r.Header.Get(HeaderScopes)),
		}, nil
	case IdentityJWT:
		tok := r.Header.Get(HeaderIdentity)
		if tok == "" {
			return validator.SecurityContext{}, errNoIdentity
		}
		c, err := verifyHS256(tok, key, now)
		if err != nil {
			return validator.SecurityContext{}, err
		}
		sec := validator.SecurityContext{Subject: c.Sub, TenantID: c.TenantID, ResourceConstraints: c.RC}
		switch s := c.Scope.(type) {
		case string:
			sec.Scopes = strings.Fields(s)
		case []any:
			for _, x := range s {
				if str, ok := x.(string); ok {
					sec.Scopes = append(sec.Scopes, str)
				}
			}
		}
		return sec, nil
	}
	return validator.SecurityContext{}, errors.New("unknown identity mode")
}

func verifyHS256(tok string, key []byte, now time.Time) (*jwtClaims, error) {
	if len(key) == 0 {
		return nil, errors.New("jwt: no verification key configured")
	}
	parts := strings.Split(tok, ".")
	if len(parts) != 3 {
		return nil, errors.New("jwt: malformed")
	}
	hdrJSON, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return nil, errors.New("jwt: bad header")
	}
	var hdr struct{ Alg string }
	if json.Unmarshal(hdrJSON, &hdr) != nil || hdr.Alg != "HS256" {
		return nil, errors.New("jwt: only HS256 is accepted")
	}
	sig, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return nil, errors.New("jwt: bad signature encoding")
	}
	mac := hmac.New(sha256.New, key)
	mac.Write([]byte(parts[0] + "." + parts[1]))
	if !hmac.Equal(sig, mac.Sum(nil)) {
		return nil, errors.New("jwt: signature mismatch")
	}
	body, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, errors.New("jwt: bad payload")
	}
	var c jwtClaims
	if err := json.Unmarshal(body, &c); err != nil {
		return nil, errors.New("jwt: bad claims")
	}
	if c.Exp == 0 || now.Unix() >= c.Exp {
		return nil, errors.New("jwt: expired or missing exp")
	}
	return &c, nil
}

// SignHS256 mints a token; used by tests and the dev CLI.
func SignHS256(claims map[string]any, key []byte) string {
	h := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"HS256","typ":"JWT"}`))
	b, _ := json.Marshal(claims)
	p := base64.RawURLEncoding.EncodeToString(b)
	mac := hmac.New(sha256.New, key)
	mac.Write([]byte(h + "." + p))
	return h + "." + p + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}
