// Package tokens provides local token estimators for the gates.
// Estimates never make network calls. Per-model correction factors are
// learned from provider-reported usage so gates drift toward real counts.
package tokens

import (
	"strings"
	"sync"

	tiktoken "github.com/pkoukk/tiktoken-go"
	loader "github.com/pkoukk/tiktoken-go-loader"
)

// Estimator returns a local token estimate for text sent to model.
type Estimator interface {
	Estimate(text string, model string) int
}

// BPE counts tokens with OpenAI's o200k_base vocabulary (embedded; no
// download). It is exact for current OpenAI models and serves as the base
// count for other providers before calibration.
type BPE struct {
	once sync.Once
	enc  *tiktoken.Tiktoken
	err  error
}

func (b *BPE) init() {
	tiktoken.SetBpeLoader(loader.NewOfflineLoader())
	b.enc, b.err = tiktoken.GetEncoding("o200k_base")
}

// Count returns the raw o200k_base token count.
func (b *BPE) Count(text string) int {
	b.once.Do(b.init)
	if b.err != nil {
		return Heuristic(text)
	}
	return len(b.enc.Encode(text, nil, nil))
}

// Heuristic is the fallback estimate: about 4 bytes per token for English
// prose and JSON.
func Heuristic(text string) int {
	return (len(text) + 3) / 4
}

// MinBytesPerToken is the conservative lower bound used by Gate 3's byte
// pre-check: a payload shorter than MinPayloadTokens × MinBytesPerToken
// bytes cannot reach MinPayloadTokens, so the estimator is skipped.
const MinBytesPerToken = 1

// Calibrated wraps a BPE counter with a per-model multiplicative correction
// learned from provider usage (an exponentially weighted moving average of
// actual/estimated).
type Calibrated struct {
	Base  *BPE
	Alpha float64 // EWMA weight of a new observation; default 0.1

	mu      sync.RWMutex
	factors map[string]float64
}

// NewCalibrated returns an estimator with optional initial factors keyed by
// model prefix (for example "claude": 1.15).
func NewCalibrated(initial map[string]float64) *Calibrated {
	f := map[string]float64{}
	for k, v := range initial {
		f[k] = v
	}
	return &Calibrated{Base: &BPE{}, Alpha: 0.1, factors: f}
}

func (c *Calibrated) Estimate(text, model string) int {
	return int(float64(c.Base.Count(text))*c.Factor(model) + 0.5)
}

// Factor returns the correction for model: an exact match, else the longest
// matching prefix, else 1.
func (c *Calibrated) Factor(model string) float64 {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if f, ok := c.factors[model]; ok {
		return f
	}
	best, bestLen := 1.0, -1
	for k, f := range c.factors {
		if strings.HasPrefix(model, k) && len(k) > bestLen {
			best, bestLen = f, len(k)
		}
	}
	return best
}

// Observe feeds back one measurement: the uncalibrated estimate for a
// request and the provider's reported input tokens.
func (c *Calibrated) Observe(model string, rawEstimate, actual int) {
	if rawEstimate <= 0 || actual <= 0 {
		return
	}
	ratio := float64(actual) / float64(rawEstimate)
	cur := c.Factor(model)
	c.mu.Lock()
	defer c.mu.Unlock()
	if _, ok := c.factors[model]; !ok {
		c.factors[model] = ratio
		return
	}
	c.factors[model] = (1-c.Alpha)*cur + c.Alpha*ratio
}
