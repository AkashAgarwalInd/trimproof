package bench

import (
	"context"
	"log"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// DriveConfig configures Drive.
type DriveConfig struct {
	Client    Client // pointed at the gateway, with the route header set
	Model     string
	Datasets  []*Dataset
	N         int // maximum requests
	RPM       int
	MaxTokens int
	// UntilEncoded stops after this many responses were served encoded
	// (the route was promoted to ENABLED); 0 means run all N.
	UntilEncoded int
}

// DriveStats summarizes a Drive run.
type DriveStats struct {
	Sent, Failed, Correct, Encoded int
}

// Drive sends benchmark questions as production traffic through a gateway
// at a fixed rate, cycling through the datasets' questions. It is used to
// exercise shadow evaluation and promotion end to end.
func Drive(ctx context.Context, cfg DriveConfig) DriveStats {
	type item struct {
		d *Dataset
		q Question
	}
	var items []item
	for _, d := range cfg.Datasets {
		for _, q := range d.Questions {
			items = append(items, item{d, q})
		}
	}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	tick := time.NewTicker(time.Minute / time.Duration(max(1, cfg.RPM)))
	defer tick.Stop()
	var (
		wg                             sync.WaitGroup
		sent, failed, correct, encoded atomic.Int64
	)
	sem := make(chan struct{}, 4)
	for i := 0; i < cfg.N && len(items) > 0; i++ {
		select {
		case <-ctx.Done():
		case <-tick.C:
		}
		if ctx.Err() != nil {
			break
		}
		it := items[i%len(items)]
		sem <- struct{}{}
		wg.Add(1)
		go func(i int) {
			defer func() { <-sem; wg.Done() }()
			sent.Add(1)
			res, err := cfg.Client.Do(ctx, Call{Model: cfg.Model, Question: it.q.Text, Tool: it.d.Tool, ToolArgs: it.d.ToolArgs,
				ToolResult: string(it.d.JSON()), MaxTokens: cfg.MaxTokens})
			if err != nil {
				if ctx.Err() == nil {
					failed.Add(1)
					log.Printf("drive %d: %v", i, err)
				}
				return
			}
			if Correct(it.q, res.Text) {
				correct.Add(1)
			}
			if !strings.HasPrefix(res.Representation, "json") && res.Representation != "" {
				if n := encoded.Add(1); cfg.UntilEncoded > 0 && int(n) >= cfg.UntilEncoded {
					log.Printf("drive: %d responses served as %q; stopping", n, res.Representation)
					cancel()
				}
			}
			if n := sent.Load(); n%25 == 0 {
				log.Printf("drive: %d sent, %d failed, %d correct, %d encoded", n, failed.Load(), correct.Load(), encoded.Load())
			}
		}(i)
	}
	wg.Wait()
	return DriveStats{Sent: int(sent.Load()), Failed: int(failed.Load()), Correct: int(correct.Load()), Encoded: int(encoded.Load())}
}
