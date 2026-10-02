// Package calibrate corrects the gates' token estimates for Claude models.
//
// The gates count tokens locally with o200k, which is exact for OpenAI
// models but not for Claude, and Claude's tokenizer may not treat JSON and
// an encoding the same way. Now and then (at most once per model per
// Interval) the Calibrator takes a data block the gates just measured and
// asks Anthropic's free count_tokens endpoint for the exact count of its
// JSON and its encoded form. The ratios become per-model, per-representation
// correction factors. Calls run in the background with the client's own
// credentials and never touch the request path.
package calibrate

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/AkashAgarwalInd/trimproof/pkg/server"
	"github.com/AkashAgarwalInd/trimproof/pkg/tokens"
)

// MaxPayloadBytes bounds the payload sent to count_tokens.
const MaxPayloadBytes = 1 << 20

// Config configures a Calibrator.
type Config struct {
	Estimator     *tokens.Calibrated
	AnthropicBase string        // e.g. https://api.anthropic.com
	Interval      time.Duration // per model; default 15 minutes
	HTTP          *http.Client
	Log           *slog.Logger
}

// Calibrator is a server.Observer.
type Calibrator struct {
	cfg   Config
	queue chan job

	mu       sync.Mutex
	last     map[string]time.Time // last calibration attempt per model
	baseline map[string]int       // count_tokens of a one-token message per model
}

type job struct {
	model, kind string
	json, enc   string
	header      http.Header
}

// New returns a Calibrator; call Run to start it.
func New(cfg Config) *Calibrator {
	if cfg.Interval <= 0 {
		cfg.Interval = 15 * time.Minute
	}
	if cfg.HTTP == nil {
		cfg.HTTP = &http.Client{Timeout: 30 * time.Second}
	}
	if cfg.Log == nil {
		cfg.Log = slog.Default()
	}
	return &Calibrator{cfg: cfg, queue: make(chan job, 4), last: map[string]time.Time{}, baseline: map[string]int{}}
}

// Observe picks the largest block of an Anthropic request that the gates
// encoded (in any state that encodes: SHADOW, ENABLED, MANUAL) when the
// model is due for calibration. It never blocks.
func (c *Calibrator) Observe(_ context.Context, x *server.Exchange) {
	if x.Adapter == nil || x.Adapter.Name() != "anthropic" || x.Original == nil || x.Decision == nil || x.Status != http.StatusOK {
		return
	}
	var j job
	for _, b := range x.Decision.Blocks {
		if b.Encoded != nil && len(b.Canonical) > len(j.json) && len(b.Canonical) <= MaxPayloadBytes {
			j.json, j.enc = string(b.Canonical), string(b.Encoded)
		}
	}
	if j.json == "" {
		return
	}
	j.model, j.kind, j.header = x.Original.Model, x.Decision.Codec, x.Header
	now := time.Now()
	c.mu.Lock()
	if t, ok := c.last[j.model]; ok && now.Sub(t) < c.cfg.Interval {
		c.mu.Unlock()
		return
	}
	c.last[j.model] = now
	c.mu.Unlock()
	select {
	case c.queue <- j:
	default:
	}
}

// Run processes calibrations until ctx is done.
func (c *Calibrator) Run(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case j := <-c.queue:
			if err := c.calibrate(ctx, j); err != nil {
				c.cfg.Log.Warn("token calibration failed", "model", j.model, "err", err)
			}
		}
	}
}

func (c *Calibrator) calibrate(ctx context.Context, j job) error {
	// count_tokens counts a whole message, including a fixed overhead.
	// A one-token message measures that overhead once per model.
	c.mu.Lock()
	base, ok := c.baseline[j.model]
	c.mu.Unlock()
	if !ok {
		n, err := c.count(ctx, j, ".")
		if err != nil {
			return err
		}
		base = n - 1
		c.mu.Lock()
		c.baseline[j.model] = base
		c.mu.Unlock()
	}
	nJSON, err := c.count(ctx, j, j.json)
	if err != nil {
		return err
	}
	nEnc, err := c.count(ctx, j, j.enc)
	if err != nil {
		return err
	}
	est := c.cfg.Estimator
	rawJSON, rawEnc := est.Base.Count(j.json), est.Base.Count(j.enc)
	est.ObserveKind(j.model, tokens.KindJSON, rawJSON, nJSON-base)
	est.ObserveKind(j.model, j.kind, rawEnc, nEnc-base)
	est.Observe(j.model, rawJSON, nJSON-base)
	c.cfg.Log.Info("token calibration", "model", j.model, "codec", j.kind,
		"json_tokens", nJSON-base, "json_o200k", rawJSON, "codec_tokens", nEnc-base, "codec_o200k", rawEnc)
	return nil
}

// count calls count_tokens for one user message holding text.
func (c *Calibrator) count(ctx context.Context, j job, text string) (int, error) {
	body, err := json.Marshal(map[string]any{
		"model":    j.model,
		"messages": []map[string]string{{"role": "user", "content": text}},
	})
	if err != nil {
		return 0, err
	}
	url := strings.TrimRight(c.cfg.AnthropicBase, "/") + "/v1/messages/count_tokens"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return 0, err
	}
	req.Header = j.header.Clone()
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.cfg.HTTP.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 1<<16))
	if err != nil {
		return 0, err
	}
	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("count_tokens: status %d: %.200s", resp.StatusCode, data)
	}
	var out struct {
		InputTokens int `json:"input_tokens"`
	}
	if err := json.Unmarshal(data, &out); err != nil {
		return 0, fmt.Errorf("count_tokens: %w", err)
	}
	return out.InputTokens, nil
}
