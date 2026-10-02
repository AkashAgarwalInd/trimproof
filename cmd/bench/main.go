// Command bench runs trimproof's benchmarks.
//
//	bench tokens                          offline o200k token table (no network)
//	bench run -targets nim:MODEL -max-calls N
//	                                      live calls; appends to -out JSONL (resumable)
//	bench report -in results.jsonl        Phase 0 report with go/no-go verdicts
//	bench verify-report -in results.jsonl verification tables with bootstrap intervals
//	bench datapoints -in results.jsonl    every measured question, arms side by side
//	bench judge -source freeform -in answers.jsonl -judge nim:MODEL -out judged.jsonl -max-calls N
//	                                      grade free-form answers blind against the data (resumable)
//	bench freeform-report -in answers.jsonl -judged judged.jsonl
//	bench dump-data -dir DIR              the generated datasets and questions as sent
//	bench fetch-wtq                       download WikiTableQuestions (CC BY-SA) to -wtq-dir
//	bench payloads [-fetch] [-held-out] [-dir DIR]    real public-API responses through the gateway's gates
//	bench drive -gateway URL -route R     production traffic through a gateway
//	                                      (exercises shadow evaluation/promotion)
//
// Targets are provider:model. Providers and their credentials:
//
//	nim        integrate.api.nvidia.com; NVIDIA_API_KEY, or OPENAI_API_KEY when
//	           OPENAI_API_BASE points at NIM
//	github     GitHub Models; GITHUB_MODELS_TOKEN
//	gemini     Gemini's OpenAI-compatible endpoint; GEMINI_API_KEY
//	openai     OPENAI_API_BASE (required) with OPENAI_API_KEY
//	anthropic  ANTHROPIC_BASE_URL (default api.anthropic.com) with ANTHROPIC_API_KEY
//
// Live calls go only to free endpoints (bench.FreeHosts) unless -allow-paid
// is given, and run refuses to start if it would make more than -max-calls
// calls.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/AkashAgarwalInd/trimproof/internal/bench"
	"github.com/AkashAgarwalInd/trimproof/internal/bench/payloads"
	"github.com/AkashAgarwalInd/trimproof/internal/bench/wtq"
)

// payloadCodec is the codec whose gates choose the payload Q&A set.
const payloadCodec = "toonx"

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
	gateway := fs.String("gateway", "http://localhost:8080/openai/v1", "gateway OpenAI base URL (drive)")
	route := fs.String("route", "", "X-Trimproof-Route value (drive)")
	model := fs.String("model", "openai/gpt-oss-20b", "model (drive)")
	n := fs.Int("n", 0, "maximum questions (run; 0 = all) or requests (drive; default 300)")
	untilEncoded := fs.Int("until-encoded", 0, "stop after this many encoded responses (drive)")
	seeds := fs.Int("seeds", 1, "number of data seeds starting at -seed (run, drive, dump-data)")
	formats := fs.String("formats", strings.Join(bench.LiveFormats, ","), "formats to send (run)")
	maxCalls := fs.Int("max-calls", 0, "refuse to start a run that would make more calls than this (run; required)")
	viaGateway := fs.String("via-gateway", "", "gateway OpenAI base URL for the gateway format (run)")
	allowPaid := fs.Bool("allow-paid", false, "allow endpoints that may bill (default: free endpoints only)")
	dir := fs.String("dir", "", "output directory (dump-data) or payload directory (payloads)")
	source := fs.String("source", "synthetic", "question source: synthetic | wtq | payloads | freeform (run, dump-data, judge)")
	wtqDir := fs.String("wtq-dir", wtq.DefaultDir(), "WikiTableQuestions release directory (fetch-wtq, -source wtq)")
	minRows := fs.Int("min-rows", 15, "smallest WTQ table to sample (-source wtq)")
	fetch := fs.Bool("fetch", false, "download the public API payloads first (payloads)")
	heldOut := fs.Bool("held-out", false, "use the held-out payload set (payloads)")
	sample := fs.Int("sample", 0, "keep this many questions, drawn with -seed and spread over kinds (run, dump-data; 0 = all)")
	judge := fs.String("judge", "", "provider:model that grades free-form answers (judge)")
	judged := fs.String("judged", "", "judge JSONL (judge: output; freeform-report: input)")
	arms := fs.String("arms", "json-compact,json-compact-2,toonx", "formats graded together (judge)")
	_ = fs.Parse(args)

	ds := bench.GenerateSeeds(split(*datasets), ints(*sizes), *seed, *seeds)
	var items []wtq.Item
	if *source == "wtq" && cmd != "fetch-wtq" {
		var err error
		if items, err = wtq.Load(*wtqDir, *n, *minRows, *seed); err != nil {
			log.Fatalf("wtq: %v (run `bench fetch-wtq` first)", err)
		}
		ds = bench.WTQDatasets(items)
	}
	var payloadItems []bench.PayloadItem
	if (*source == "payloads" || *source == "freeform") && cmd != "payloads" {
		var err error
		sets := map[string]string{"tuning": payloads.DefaultDir(), "held-out": payloads.HeldOutDir()}
		if ds, payloadItems, err = bench.PayloadDatasets(sets, payloadCodec, *seed); err != nil {
			log.Fatalf("payloads: %v (fetch them with `bench payloads -fetch` and `-fetch -held-out`)", err)
		}
		if *source == "freeform" {
			ds, payloadItems = bench.FreeFormDatasets(ds, payloadItems, *seed)
		}
	}
	if *source != "synthetic" && *source != "wtq" && *source != "payloads" && *source != "freeform" {
		log.Fatalf("unknown -source %q", *source)
	}
	ds = bench.SampleQuestions(ds, *sample, *seed)
	// manifest records which WTQ items or payloads a run or dump used.
	manifest := func(path string) {
		if items != nil {
			if err := bench.WriteWTQManifest(path, *wtqDir, items, *n, *minRows, *seed); err != nil {
				log.Fatal(err)
			}
		}
		if payloadItems != nil {
			if err := bench.WritePayloadManifest(path, payloadCodec, *seed, payloadItems); err != nil {
				log.Fatal(err)
			}
		}
	}

	switch cmd {
	case "tokens":
		if err := bench.TokenReport(os.Stdout, ds); err != nil {
			log.Fatal(err)
		}
	case "run":
		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
		defer stop()
		ts, err := parseTargets(*targets, *allowPaid)
		if err != nil {
			log.Fatal(err)
		}
		if *maxCalls <= 0 {
			log.Fatal("run: -max-calls is required")
		}
		if err := os.MkdirAll(dirOf(*out), 0o755); err != nil {
			log.Fatal(err)
		}
		cfg := bench.RunConfig{Targets: ts, Datasets: bench.LimitQuestions(ds, *n), Formats: split(*formats), Out: *out,
			Concurrency: *conc, RPM: *rpm, MaxTokens: *maxTok, MaxCalls: *maxCalls}
		if *viaGateway != "" {
			if len(ts) != 1 {
				log.Fatal("run: -via-gateway takes exactly one target")
			}
			if err := bench.CheckEndpoint(*viaGateway, *allowPaid); err != nil {
				log.Fatal(err)
			}
			// The gateway forwards the target's own key to its upstream.
			key := ts[0].Client.(*bench.OpenAI).APIKey
			cfg.Gateway = &bench.OpenAI{BaseURL: strings.TrimRight(*viaGateway, "/"), APIKey: key,
				HTTP: &http.Client{Timeout: 6 * time.Minute}, Header: map[string]string{"X-Trimproof-Route": *route}}
		}
		manifest(strings.TrimSuffix(*out, ".jsonl") + ".manifest.json")
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
	case "verify-report", "datapoints":
		recs, err := bench.ReadRecords(*in)
		if err != nil {
			log.Fatal(err)
		}
		if cmd == "datapoints" {
			bench.DatapointsReport(os.Stdout, recs)
		} else {
			bench.VerifyReport(os.Stdout, recs, bench.DefaultVerify())
		}
	case "judge":
		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
		defer stop()
		ts, err := parseTargets(*judge, *allowPaid)
		if err != nil {
			log.Fatal(err)
		}
		if len(ts) != 1 || *judged == "" || *maxCalls <= 0 {
			log.Fatal("judge: needs one -judge target, -judged and -max-calls")
		}
		recs, err := bench.ReadRecords(*in)
		if err != nil {
			log.Fatal(err)
		}
		if err := bench.Judge(ctx, bench.JudgeConfig{Client: ts[0].Client, Model: ts[0].Model, Datasets: ds, Records: recs,
			Arms: split(*arms), Out: *judged, Seed: *seed, MaxCalls: *maxCalls, Concurrency: *conc, RPM: *rpm,
			MaxTokens: *maxTok}); err != nil {
			log.Fatal(err)
		}
	case "freeform-report":
		recs, err := bench.ReadRecords(*in)
		if err != nil {
			log.Fatal(err)
		}
		js, err := bench.ReadJudged(*judged)
		if err != nil {
			log.Fatal(err)
		}
		bench.FreeFormReport(os.Stdout, recs, js, bench.DefaultVerify())
	case "dump-data":
		if *dir == "" {
			log.Fatal("dump-data: -dir is required")
		}
		if err := bench.DumpData(*dir, ds); err != nil {
			log.Fatal(err)
		}
		manifest(filepath.Join(*dir, *source+"-manifest.json"))
	case "payloads":
		sources := payloads.Sources
		if *heldOut {
			sources = payloads.HeldOut
		}
		if *dir == "" {
			*dir = payloads.DefaultDir()
			if *heldOut {
				*dir = payloads.HeldOutDir()
			}
		}
		if *fetch {
			if _, err := payloads.Fetch(context.Background(), *dir, sources); err != nil {
				log.Fatal(err)
			}
		}
		if err := payloads.Analyze(*dir, os.Stdout); err != nil {
			log.Fatalf("payloads: %v (fetch them with -fetch)", err)
		}
	case "fetch-wtq":
		if err := wtq.Fetch(context.Background(), *wtqDir); err != nil {
			log.Fatal(err)
		}
		fmt.Println("WikiTableQuestions release in", *wtqDir)
	case "drive":
		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
		defer stop()
		if err := bench.CheckEndpoint(*gateway, *allowPaid); err != nil {
			log.Fatal(err)
		}
		if *n == 0 {
			*n = 300
		}
		c := &bench.OpenAI{BaseURL: strings.TrimRight(*gateway, "/"), APIKey: os.Getenv("OPENAI_API_KEY"),
			HTTP: &http.Client{Timeout: 6 * time.Minute}, Header: map[string]string{"X-Trimproof-Route": *route}}
		st := bench.Drive(ctx, bench.DriveConfig{Client: c, Model: *model, Datasets: ds, N: *n, RPM: *rpm,
			MaxTokens: *maxTok, UntilEncoded: *untilEncoded})
		fmt.Printf("sent %d, failed %d, correct %d, served encoded %d\n", st.Sent, st.Failed, st.Correct, st.Encoded)
	default:
		log.Fatalf("unknown command %q", cmd)
	}
}

func parseTargets(s string, allowPaid bool) ([]bench.Target, error) {
	hc := &http.Client{Timeout: 6 * time.Minute}
	var out []bench.Target
	for _, t := range split(s) {
		prov, model, ok := strings.Cut(t, ":")
		if !ok {
			return nil, fmt.Errorf("target %q: want provider:model", t)
		}
		var base, key string
		switch prov {
		case "nim":
			base, key = "https://integrate.api.nvidia.com/v1", os.Getenv("NVIDIA_API_KEY")
			// Reuse OPENAI_API_KEY only when it is already a NIM key, so a
			// real OpenAI key is never sent to another provider.
			if key == "" && strings.Contains(os.Getenv("OPENAI_API_BASE"), "integrate.api.nvidia.com") {
				key = os.Getenv("OPENAI_API_KEY")
			}
		case "github":
			base, key = "https://models.github.ai/inference", os.Getenv("GITHUB_MODELS_TOKEN")
		case "gemini":
			base, key = "https://generativelanguage.googleapis.com/v1beta/openai", os.Getenv("GEMINI_API_KEY")
		case "openai":
			base, key = os.Getenv("OPENAI_API_BASE"), os.Getenv("OPENAI_API_KEY")
			if base == "" {
				return nil, fmt.Errorf("target %q: set OPENAI_API_BASE (there is no default endpoint)", t)
			}
		case "anthropic":
			base, key = os.Getenv("ANTHROPIC_BASE_URL"), os.Getenv("ANTHROPIC_API_KEY")
			if base == "" {
				base = "https://api.anthropic.com"
			}
		default:
			return nil, fmt.Errorf("unknown provider %q", prov)
		}
		if err := bench.CheckEndpoint(base, allowPaid); err != nil {
			return nil, fmt.Errorf("target %q: %w", t, err)
		}
		if key == "" {
			return nil, fmt.Errorf("target %q: no API key in the environment", t)
		}
		base = strings.TrimRight(base, "/")
		var c bench.Client = &bench.OpenAI{BaseURL: base, APIKey: key, HTTP: hc}
		if prov == "anthropic" {
			c = &bench.Anthropic{BaseURL: base, APIKey: key, HTTP: hc}
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
