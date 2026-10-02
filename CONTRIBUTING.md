# Contributing

Issues and pull requests are welcome. trimproof is pre-alpha, so open an issue first for anything larger than a fix, so the design can be agreed before you write the code.

## Build and test

```sh
go build ./...
go vet ./...
go test -race ./...
```

CI also checks `gofmt`, runs `govulncheck`, and runs short fuzz targets:

```sh
go test ./pkg/codec/codectest -run '^$' -fuzz '^FuzzRoundTrip$' -fuzztime 30s
go test ./pkg/canonical -run '^$' -fuzz '^FuzzIdempotent$' -fuzztime 30s
go test ./pkg/tokens -run '^$' -fuzz '^FuzzO200k$' -fuzztime 30s
```

## Ground rules

- **Codecs must be lossless.** A new codec must satisfy `Decode(Encode(x)) == canonical(x)` byte for byte, and be imported by `pkg/codec/codectest`, so the round-trip fuzz target covers it. A codec that can't represent a payload exactly must refuse it, not approximate it.
- **Ineligible traffic stays byte-identical.** Anything the gates reject is forwarded exactly as received. Tests in `pkg/server` check this; keep them passing.
- **Claims need data.** Changes to the promotion rule or its defaults should come with the simulation or measurement behind them, the way [`bench/results/PROMOTION.md`](bench/results/PROMOTION.md) does.

## Benchmark results from other models

The most useful contribution right now is data: results on models and providers that haven't been run yet, especially Claude and GPT-4-class models.

```sh
go run ./cmd/bench run -targets openai:MODEL,anthropic:MODEL
go run ./cmd/bench report -in bench/results/<file>.jsonl
```

Open an issue with the "Benchmark results" template and paste the report.

## License

By contributing, you agree that your contributions are licensed under the Apache License 2.0, as the rest of the project is.
