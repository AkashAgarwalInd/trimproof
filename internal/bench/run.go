package bench

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"math/rand/v2"
	"os"
	"slices"
	"strings"
	"sync"
	"time"
)

// Record is one live benchmark observation (one JSONL line).
type Record struct {
	Provider     string `json:"provider"`
	Model        string `json:"model"`
	Dataset      string `json:"dataset"`
	Rows         int    `json:"rows"`
	Seed         uint64 `json:"seed,omitempty"`
	QuestionID   string `json:"question_id"`
	Kind         string `json:"kind"`
	Question     string `json:"question,omitempty"`
	Format       string `json:"format"`
	Answer       string `json:"answer"`
	Reply        string `json:"reply"`
	Correct      bool   `json:"correct"`
	InputTokens  int    `json:"input_tokens"`
	OutputTokens int    `json:"output_tokens"`
	// ReasoningTokens is the part of OutputTokens spent on reasoning
	// (estimated from the reasoning text when ReasoningEstimated is set).
	ReasoningTokens    int   `json:"reasoning_tokens,omitempty"`
	ReasoningEstimated bool  `json:"reasoning_estimated,omitempty"`
	LatencyMS          int64 `json:"latency_ms"`
	// Representation is the gateway's X-Trimproof-Representation header
	// (gateway arm only).
	Representation string `json:"representation,omitempty"`
	// Empty is set when the provider returned no answer text even after
	// retries. Such calls count as incorrect and are reported per format.
	Empty bool   `json:"empty,omitempty"`
	Error string `json:"error,omitempty"`
}

func (r Record) key() string {
	return r.Provider + "|" + r.Model + "|" + r.QuestionID + "|" + r.Format
}

// Target is a provider/model pair to benchmark.
type Target struct {
	Provider string
	Model    string
	Client   Client
}

// RunConfig controls a live run.
type RunConfig struct {
	Targets     []Target
	Datasets    []*Dataset
	Formats     []string
	Out         string // JSONL path; existing successful records are skipped
	Concurrency int
	RPM         int // requests per minute across all targets
	MaxTokens   int
	// MaxCalls caps the calls one invocation may plan; a run that would
	// exceed it does not start. Empty replies are retried up to twice more.
	MaxCalls int
	// Gateway sends the "gateway" format: JSON through a trimproof gateway
	// on a route that encodes it.
	Gateway Client
}

// Run executes all (target, question, format) calls not already in Out.
func Run(ctx context.Context, cfg RunConfig) error {
	done := map[string]bool{}
	if prev, err := ReadRecords(cfg.Out); err == nil {
		for _, r := range prev {
			if r.Error == "" {
				done[r.key()] = true
			}
		}
	}
	f, err := os.OpenFile(cfg.Out, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()

	type job struct {
		t      Target
		d      *Dataset
		q      Question
		format string
	}
	var jobs []job
	// Interleave formats per question so all arms of a pair run close in time.
	for _, d := range cfg.Datasets {
		for _, q := range d.Questions {
			for _, t := range cfg.Targets {
				for _, fm := range cfg.Formats {
					r := Record{Provider: t.Provider, Model: t.Model, QuestionID: q.ID, Format: fm}
					if !done[r.key()] {
						jobs = append(jobs, job{t, d, q, fm})
					}
				}
			}
		}
	}
	log.Printf("bench: %d calls to make (%d already done)", len(jobs), len(done))
	if len(jobs) == 0 {
		return nil
	}
	if len(jobs) > cfg.MaxCalls {
		return fmt.Errorf("bench: %d calls planned, more than -max-calls %d; nothing was sent", len(jobs), cfg.MaxCalls)
	}
	if cfg.Gateway == nil && slices.Contains(cfg.Formats, "gateway") {
		return fmt.Errorf("bench: the gateway format needs -via-gateway")
	}

	interval := time.Minute / time.Duration(max(1, cfg.RPM))
	tick := time.NewTicker(interval)
	defer tick.Stop()
	sem := make(chan struct{}, max(1, cfg.Concurrency))
	var mu sync.Mutex
	var wg sync.WaitGroup
	w := bufio.NewWriter(f)
	completed, failed := 0, 0

	rendered := map[string][2]string{}
	render := func(d *Dataset, fm string) (string, string, error) {
		k := d.Key() + "|" + fm
		mu.Lock()
		defer mu.Unlock()
		if v, ok := rendered[k]; ok {
			return v[0], v[1], nil
		}
		text, primer, err := Render(fm, d)
		if err == nil {
			rendered[k] = [2]string{text, primer}
		}
		return text, primer, err
	}

	for _, j := range jobs {
		select {
		case <-ctx.Done():
			wg.Wait()
			return ctx.Err()
		case <-tick.C:
		}
		sem <- struct{}{}
		wg.Add(1)
		go func(j job) {
			defer wg.Done()
			defer func() { <-sem }()
			rec := Record{Provider: j.t.Provider, Model: j.t.Model, Dataset: j.d.Name, Rows: j.d.Rows, Seed: j.d.Seed,
				QuestionID: j.q.ID, Kind: j.q.Kind, Question: j.q.Text, Format: j.format, Answer: j.q.Answer}
			if len(j.q.Answers) > 0 {
				rec.Answer = strings.Join(j.q.Answers, " | ")
			}
			client := j.t.Client
			if j.format == "gateway" {
				client = cfg.Gateway
			}
			text, primer, err := render(j.d, j.format)
			if err == nil {
				var res *Result
				// Some serving stacks intermittently return empty content;
				// retry so a transport artifact is not scored as a format effect.
				for attempt := 0; attempt < 3; attempt++ {
					cctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
					res, err = client.Do(cctx, Call{
						Model: j.t.Model, System: systemFor(j.q), Primer: primer, Question: j.q.Text,
						Tool: j.d.Tool, ToolArgs: j.d.ToolArgs, ToolResult: text, MaxTokens: cfg.MaxTokens,
					})
					cancel()
					if err != nil || strings.TrimSpace(Clean(res.Text)) != "" {
						break
					}
				}
				if err == nil {
					rec.Empty = strings.TrimSpace(Clean(res.Text)) == ""
					rec.Reply, rec.InputTokens, rec.OutputTokens = res.Text, res.InputTokens, res.OutputTokens
					rec.ReasoningTokens, rec.ReasoningEstimated = res.ReasoningTokens, res.ReasoningEstimated
					rec.LatencyMS = res.Latency.Milliseconds()
					rec.Representation = res.Representation
					if !j.q.FreeForm {
						rec.Correct = Correct(j.q, res.Text)
					}
				}
			}
			if err != nil {
				rec.Error = err.Error()
			}
			line, _ := json.Marshal(rec)
			mu.Lock()
			defer mu.Unlock()
			w.Write(append(line, '\n'))
			w.Flush()
			completed++
			if err != nil {
				failed++
				log.Printf("bench: %s %s %s: %v", j.t.Model, j.q.ID, j.format, err)
			}
			if completed%25 == 0 || completed == len(jobs) {
				log.Printf("bench: %d/%d done (%d errors)", completed, len(jobs), failed)
			}
		}(j)
	}
	wg.Wait()
	return nil
}

// ReadRecords loads a JSONL results file.
func ReadRecords(path string) ([]Record, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var out []Record
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 16<<20)
	for sc.Scan() {
		var r Record
		if err := json.Unmarshal(sc.Bytes(), &r); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, sc.Err()
}

// SampleQuestions keeps n questions across datasets, drawn at random with
// seed and spread over question kinds: it takes one question of each kind in
// turn until n are taken. Datasets left without questions are dropped; n <= 0
// keeps all.
func SampleQuestions(ds []*Dataset, n int, seed uint64) []*Dataset {
	if n <= 0 {
		return ds
	}
	type pick struct{ d, q int }
	byKind := map[string][]pick{}
	var kinds []string
	for i, d := range ds {
		for j, q := range d.Questions {
			if byKind[q.Kind] == nil {
				kinds = append(kinds, q.Kind)
			}
			byKind[q.Kind] = append(byKind[q.Kind], pick{i, j})
		}
	}
	r := rand.New(rand.NewPCG(seed, 0x5a3b1e))
	for _, k := range kinds {
		r.Shuffle(len(byKind[k]), func(a, b int) { byKind[k][a], byKind[k][b] = byKind[k][b], byKind[k][a] })
	}
	keep := map[pick]bool{}
	for left := true; left && len(keep) < n; {
		left = false
		for _, k := range kinds {
			if len(byKind[k]) > 0 && len(keep) < n {
				keep[byKind[k][0]], byKind[k], left = true, byKind[k][1:], true
			}
		}
	}
	var out []*Dataset
	for i, d := range ds {
		c := *d
		c.Questions = nil
		for j, q := range d.Questions {
			if keep[pick{i, j}] {
				c.Questions = append(c.Questions, q)
			}
		}
		if len(c.Questions) > 0 {
			out = append(out, &c)
		}
	}
	return out
}

// LimitQuestions keeps the first n questions across datasets, in order;
// n <= 0 keeps all.
func LimitQuestions(ds []*Dataset, n int) []*Dataset {
	if n <= 0 {
		return ds
	}
	var out []*Dataset
	for _, d := range ds {
		if n == 0 {
			break
		}
		c := *d
		c.Questions = d.Questions[:min(n, len(d.Questions))]
		n -= len(c.Questions)
		out = append(out, &c)
	}
	return out
}
