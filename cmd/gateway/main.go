// Command gateway runs the trimproof LLM gateway.
//
//	gateway -policies policies.json -listen :8080
//
// Clients point their SDK base URL at the gateway:
//
//	Anthropic: http://gateway:8080/anthropic   (POST /v1/messages is intercepted)
//	OpenAI:    http://gateway:8080/openai/v1   (POST /chat/completions is intercepted)
//
// and send X-Trimproof-Route plus identity (X-TP-Identity JWT) from
// trusted ingress. Provider API keys pass through from the client.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"

	"github.com/AkashAgarwalInd/trimproof/pkg/audit"
	_ "github.com/AkashAgarwalInd/trimproof/pkg/codec/tabular"
	_ "github.com/AkashAgarwalInd/trimproof/pkg/codec/toon"
	"github.com/AkashAgarwalInd/trimproof/pkg/eval"
	"github.com/AkashAgarwalInd/trimproof/pkg/server"
	"github.com/AkashAgarwalInd/trimproof/pkg/tokens"
)

// version is set at build time with -ldflags "-X main.version=...".
var version = "dev"

func main() {
	listen := flag.String("listen", ":8080", "listen address")
	policies := flag.String("policies", "policies.json", "route policy file")
	anthropicBase := flag.String("anthropic-base", "https://api.anthropic.com", "Anthropic API base URL")
	openaiBase := flag.String("openai-base", envOr("OPENAI_API_BASE", "https://api.openai.com/v1"), "OpenAI-compatible base URL (including /v1)")
	identityMode := flag.String("identity-mode", string(server.IdentityJWT), "jwt-hs256 | trusted-headers")
	identityKeyEnv := flag.String("identity-key-env", "TP_IDENTITY_KEY", "env var holding the HS256 identity key")
	requireIdentity := flag.Bool("require-identity", true, "reject requests without identity")
	pairsFile := flag.String("pairs-file", "eval-pairs.jsonl", "shadow evaluation pairs (JSONL)")
	auditFile := flag.String("audit-file", "audit.jsonl", "audit log (JSONL); empty disables")
	transitionsFile := flag.String("transitions-file", "transitions.jsonl", "promotion transitions (JSONL)")
	evalRPS := flag.Float64("eval-rps", 1, "per-provider evaluation request rate")
	showVersion := flag.Bool("version", false, "print version and exit")
	flag.Parse()
	if *showVersion {
		fmt.Println("trimproof-gateway", version)
		return
	}

	log := slog.New(slog.NewJSONHandler(os.Stderr, nil))
	slog.SetDefault(log)
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// OpenTelemetry metrics via OTLP/HTTP when OTEL_EXPORTER_OTLP_ENDPOINT is set.
	if os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT") != "" {
		exp, err := otlpmetrichttp.New(ctx)
		if err != nil {
			fatal(log, "otlp exporter", err)
		}
		mp := sdkmetric.NewMeterProvider(sdkmetric.WithReader(sdkmetric.NewPeriodicReader(exp)))
		otel.SetMeterProvider(mp)
		defer mp.Shutdown(context.Background())
	}

	reg, err := server.LoadRegistry(*policies)
	if err != nil {
		fatal(log, "load policies", err)
	}
	if n, err := eval.ReplayTransitions(*transitionsFile, reg); err != nil {
		fatal(log, "replay transitions", err)
	} else if n > 0 {
		log.Info("restored promotion state", "routes", n)
	}
	metrics, err := audit.NewMetrics()
	if err != nil {
		fatal(log, "metrics", err)
	}
	store, err := eval.NewMemoryStore(*pairsFile)
	if err != nil {
		fatal(log, "pair store", err)
	}
	defer store.Close()
	transitions := openAppend(log, *transitionsFile)
	defer transitions.Close()
	prom := &eval.Promoter{Registry: reg, Store: store, T: eval.DefaultThresholds(), OnTransition: func(tr eval.Transition) {
		metrics.Transition(tr)
		b, _ := json.Marshal(tr)
		transitions.Write(append(b, '\n'))
		log.Info("route state changed", "route", tr.TenantID+"/"+tr.RouteID, "from", tr.From.String(), "to", tr.To.String(), "reason", tr.Reason)
	}}
	ev := eval.NewEvaluator(eval.Config{Store: store, Promoter: prom, RPS: *evalRPS, Log: log})
	go ev.Run(ctx)

	observers := []server.Observer{metrics, prom, ev}
	if *auditFile != "" {
		sink, err := audit.NewJSONLFile(*auditFile)
		if err != nil {
			fatal(log, "audit file", err)
		}
		defer sink.Close()
		aud := audit.New(audit.Config{Sinks: []audit.Sink{sink}, Log: log})
		defer aud.Close()
		observers = append(observers, aud)
	}

	gw := server.New(server.Config{
		Registry: reg, AnthropicBase: *anthropicBase, OpenAIBase: *openaiBase,
		IdentityMode: server.IdentityMode(*identityMode), IdentityKey: []byte(os.Getenv(*identityKeyEnv)),
		RequireIdentity: *requireIdentity, Estimator: tokens.NewCalibrated(nil), Observers: observers, Log: log,
	})
	srv := &http.Server{Addr: *listen, Handler: gw, ReadHeaderTimeout: 10 * time.Second}
	go func() {
		<-ctx.Done()
		shutdown, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		srv.Shutdown(shutdown)
	}()
	log.Info("trimproof gateway listening", "addr", *listen, "version", version, "policies", *policies)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		fatal(log, "serve", err)
	}
}

func envOr(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func openAppend(log *slog.Logger, path string) *os.File {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		fatal(log, "open "+path, err)
	}
	return f
}

func fatal(log *slog.Logger, msg string, err error) {
	log.Error(msg, "err", err)
	os.Exit(1)
}
