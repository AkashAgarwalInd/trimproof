// Command bench runs the Phase 0 kill-or-go benchmark (spec §12).
//
//	bench tokens                       offline o200k token table (no network)
//	bench run -targets openai:MODEL    live calls; appends to -out JSONL (resumable)
//	bench report -in results.jsonl     markdown report with go/no-go verdicts
//
// Provider credentials come from the environment: OPENAI_API_KEY and
// OPENAI_API_BASE (any OpenAI-compatible endpoint), ANTHROPIC_API_KEY and
// optional ANTHROPIC_BASE_URL.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"time"

	"github.com/AkashAgarwalInd/trimproof/internal/bench"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: bench tokens|run|report [flags]")
		os.Exit(2)
	}
	cmd, args := os.Args[1], os.Args[2:]
	fs := flag.NewFlagSet(cmd, flag.ExitOnError)
	datasets := fs.String("datasets", "orders,logs,search,employees", "comma-separated datasets")
	sizes := fs.String("sizes", "30,120", "comma-separated row counts")
	seed := fs.Uint64("seed", 42, "data generator seed")
	targets := fs.String("targets", "", "comma-separated provider:model (provider = openai|anthropic)")
	out := fs.String("out", "bench/results/results.jsonl", "results JSONL (run)")
	in := fs.String("in", "bench/results/results.jsonl", "results JSONL (report)")
	conc := fs.Int("concurrency", 6, "max in-flight requests")
	rpm := fs.Int("rpm", 30, "max requests per minute")
	maxTok := fs.Int("max-tokens", 4096, "max output tokens (reasoning models need headroom)")
	minRed := fs.Float64("min-reduction", 0.20, "go criterion: minimum input-token reduction")
	alpha := fs.Float64("alpha", 0.05, "McNemar significance level")
	_ = fs.Parse(args)

	ds := bench.Generate(split(*datasets), ints(*sizes), *seed)

	switch cmd {
	case "tokens":
		if err := bench.TokenReport(os.Stdout, ds); err != nil {
			log.Fatal(err)
		}
	case "run":
		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
		defer stop()
		ts, err := parseTargets(*targets)
		if err != nil {
			log.Fatal(err)
		}
		if err := os.MkdirAll(dirOf(*out), 0o755); err != nil {
			log.Fatal(err)
		}
		cfg := bench.RunConfig{Targets: ts, Datasets: ds, Formats: bench.LiveFormats, Out: *out,
			Concurrency: *conc, RPM: *rpm, MaxTokens: *maxTok}
		if err := bench.Run(ctx, cfg); err != nil {
			log.Fatal(err)
		}
	case "report":
		recs, err := bench.ReadRecords(*in)
		if err != nil {
			log.Fatal(err)
		}
		fmt.Println("# trimproof Phase 0 benchmark")
		fmt.Println()
		if err := bench.TokenReport(os.Stdout, ds); err != nil {
			log.Fatal(err)
		}
		bench.LiveReport(os.Stdout, recs, bench.GoCriteria{MinReduction: *minRed, Alpha: *alpha})
	default:
		log.Fatalf("unknown command %q", cmd)
	}
}

func parseTargets(s string) ([]bench.Target, error) {
	hc := &http.Client{Timeout: 6 * time.Minute}
	var out []bench.Target
	for _, t := range split(s) {
		prov, model, ok := strings.Cut(t, ":")
		if !ok {
			return nil, fmt.Errorf("target %q: want provider:model", t)
		}
		var c bench.Client
		switch prov {
		case "openai":
			base := os.Getenv("OPENAI_API_BASE")
			if base == "" {
				base = "https://api.openai.com/v1"
			}
			c = &bench.OpenAI{BaseURL: strings.TrimRight(base, "/"), APIKey: os.Getenv("OPENAI_API_KEY"), HTTP: hc}
		case "anthropic":
			base := os.Getenv("ANTHROPIC_BASE_URL")
			if base == "" {
				base = "https://api.anthropic.com"
			}
			c = &bench.Anthropic{BaseURL: strings.TrimRight(base, "/"), APIKey: os.Getenv("ANTHROPIC_API_KEY"), HTTP: hc}
		default:
			return nil, fmt.Errorf("unknown provider %q", prov)
		}
		out = append(out, bench.Target{Provider: prov, Model: model, Client: c})
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("no -targets given")
	}
	return out, nil
}

func split(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func ints(s string) []int {
	var out []int
	for _, p := range split(s) {
		n, err := strconv.Atoi(p)
		if err != nil {
			log.Fatalf("bad size %q", p)
		}
		out = append(out, n)
	}
	return out
}

func dirOf(p string) string {
	if i := strings.LastIndex(p, "/"); i > 0 {
		return p[:i]
	}
	return "."
}
