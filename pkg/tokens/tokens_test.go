package tokens

import "testing"

func TestBPECount(t *testing.T) {
	var b BPE
	if n := b.Count("hello world"); n != 2 {
		t.Fatalf("o200k 'hello world' = %d tokens, want 2", n)
	}
}

func TestCalibration(t *testing.T) {
	c := NewCalibrated(map[string]float64{"claude": 1.2})
	if f := c.Factor("claude-sonnet-5-5"); f != 1.2 {
		t.Fatalf("prefix factor = %v", f)
	}
	if f := c.Factor("gpt-x"); f != 1 {
		t.Fatalf("default factor = %v", f)
	}
	c.Observe("claude-sonnet-5-5", 100, 150) // first exact observation is taken as-is
	if f := c.Factor("claude-sonnet-5-5"); f != 1.5 {
		t.Fatalf("after first observation = %v", f)
	}
	c.Observe("claude-sonnet-5-5", 100, 100)
	if f := c.Factor("claude-sonnet-5-5"); f < 1.449 || f > 1.451 {
		t.Fatalf("EWMA = %v, want 1.45", f)
	}
}

func TestCalibrationPerKind(t *testing.T) {
	c := NewCalibrated(nil)
	c.Observe("claude-x", 100, 120)
	if f := c.FactorKind("claude-x", "toon"); f != 1.2 {
		t.Fatalf("kind falls back to model factor: %v", f)
	}
	c.ObserveKind("claude-x", "toon", 100, 140)
	c.ObserveKind("claude-x", KindJSON, 100, 110)
	if f := c.FactorKind("claude-x", "toon"); f != 1.4 {
		t.Fatalf("toon factor %v", f)
	}
	if f := c.FactorKind("claude-x", KindJSON); f != 1.1 {
		t.Fatalf("json factor %v", f)
	}
	if f := c.Factor("claude-x"); f != 1.2 {
		t.Fatalf("model factor changed: %v", f)
	}
	if got := c.Factors(); got["claude-x/toon"] != 1.4 || got["claude-x"] != 1.2 {
		t.Fatalf("Factors() = %v", got)
	}
	var est Estimator = c
	if EstimateKind(est, "hello world", "claude-x", "toon") != 3 { // 2 × 1.4 = 2.8
		t.Fatal("EstimateKind did not use the kind factor")
	}
}
