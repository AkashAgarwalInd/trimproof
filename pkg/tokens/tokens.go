// Package tokens provides local token estimators for the gates.
// Estimates never make network calls. Correction factors per model, and
// per model and representation, can be fed from exact provider counts (see
// package calibrate) so the gates drift toward the provider's tokenizer.
package tokens

import (
	"strings"
	"sync"
	"unicode/utf8"

	loader "github.com/pkoukk/tiktoken-go-loader"
)

// Estimator returns a local token estimate for text sent to model.
type Estimator interface {
	Estimate(text string, model string) int
}

// KindEstimator is an Estimator that can correct per representation:
// kind is "json" or a codec name. A provider's tokenizer can treat JSON and
// an encoding differently from o200k, which would bias Gate 4.
type KindEstimator interface {
	Estimator
	EstimateKind(text, model, kind string) int
}

// KindJSON is the representation kind of compact canonical JSON.
const KindJSON = "json"

// EstimateKind uses est's per-representation correction when it has one.
func EstimateKind(est Estimator, text, model, kind string) int {
	if ke, ok := est.(KindEstimator); ok {
		return ke.EstimateKind(text, model, kind)
	}
	return est.Estimate(text, model)
}

// BPE counts tokens with OpenAI's o200k_base vocabulary (embedded; no
// download). It is exact for current OpenAI models and serves as the base
// count for other providers before calibration. Special-token strings are
// counted as ordinary text.
type BPE struct {
	once  sync.Once
	ranks map[string]int
	err   error
}

const o200kFile = "https://openaipublic.blob.core.windows.net/encodings/o200k_base.tiktoken"

func (b *BPE) init() {
	// The offline loader serves the embedded vocabulary by its URL.
	b.ranks, b.err = loader.NewOfflineLoader().LoadTiktokenBpe(o200kFile)
}

// Count returns the raw o200k_base token count.
func (b *BPE) Count(text string) int {
	b.once.Do(b.init)
	if b.err != nil {
		return Heuristic(text)
	}
	if !utf8.ValidString(text) {
		// tiktoken decodes to runes first: each invalid byte becomes U+FFFD.
		text = string([]rune(text))
	}
	return countO200k(text, b.ranks)
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

// Calibrated wraps a BPE counter with multiplicative corrections per model
// and per (model, representation), each an exponentially weighted moving
// average of actual/estimated.
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

// EstimateKind implements KindEstimator.
func (c *Calibrated) EstimateKind(text, model, kind string) int {
	return int(float64(c.Base.Count(text))*c.FactorKind(model, kind) + 0.5)
}

// FactorKind returns the correction for model and representation kind,
// falling back to Factor(model).
func (c *Calibrated) FactorKind(model, kind string) float64 {
	c.mu.RLock()
	f, ok := c.factors[kindKey(model, kind)]
	c.mu.RUnlock()
	if ok {
		return f
	}
	return c.Factor(model)
}

func kindKey(model, kind string) string { return model + "\x00" + kind }

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

// Observe feeds back one measurement for model: the uncalibrated (raw
// o200k) count of a text and the provider's exact count of it.
func (c *Calibrated) Observe(model string, rawEstimate, actual int) {
	c.observe(model, rawEstimate, actual)
}

// ObserveKind is Observe for one representation kind.
func (c *Calibrated) ObserveKind(model, kind string, rawEstimate, actual int) {
	c.observe(kindKey(model, kind), rawEstimate, actual)
}

// Factors returns a copy of the learned factors, keyed "model" or
// "model/kind".
func (c *Calibrated) Factors() map[string]float64 {
	c.mu.RLock()
	defer c.mu.RUnlock()
	out := make(map[string]float64, len(c.factors))
	for k, f := range c.factors {
		out[strings.Replace(k, "\x00", "/", 1)] = f
	}
	return out
}

func (c *Calibrated) observe(key string, rawEstimate, actual int) {
	if rawEstimate <= 0 || actual <= 0 {
		return
	}
	ratio := float64(actual) / float64(rawEstimate)
	c.mu.Lock()
	defer c.mu.Unlock()
	cur, ok := c.factors[key]
	if !ok {
		// The first exact measurement replaces any prefix default.
		c.factors[key] = ratio
		return
	}
	c.factors[key] = (1-c.Alpha)*cur + c.Alpha*ratio
}
