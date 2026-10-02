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
