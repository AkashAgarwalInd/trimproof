package bench

import "testing"

func TestCheckEndpoint(t *testing.T) {
	for _, c := range []struct {
		base      string
		allowPaid bool
		ok        bool
	}{
		{"https://integrate.api.nvidia.com/v1", false, true},
		{"https://models.github.ai/inference", false, true},
		{"https://generativelanguage.googleapis.com/v1beta/openai", false, true},
		{"http://127.0.0.1:8080/openai/v1", false, true},
		{"http://localhost:8080/openai/v1", false, true},
		{"https://api.openai.com/v1", false, false},
		{"https://api.anthropic.com", false, false},
		{"https://integrate.api.nvidia.com.evil.example/v1", false, false},
		{"https://api.openai.com/v1", true, true},
		{"", false, false},
		{"integrate.api.nvidia.com", false, false}, // no scheme: no host
	} {
		if err := CheckEndpoint(c.base, c.allowPaid); (err == nil) != c.ok {
			t.Errorf("CheckEndpoint(%q, %v) = %v, want ok=%v", c.base, c.allowPaid, err, c.ok)
		}
	}
}
