# trimproof

[![ci](https://github.com/AkashAgarwalInd/trimproof/actions/workflows/ci.yml/badge.svg)](https://github.com/AkashAgarwalInd/trimproof/actions/workflows/ci.yml)

**The measured, lossless context optimizer for LLM traffic.**

trimproof is a Go gateway for the Anthropic Messages and OpenAI-compatible Chat Completions APIs. It re-encodes tabular tool results (DB rows, logs, search hits) into token-efficient formats: [TOON](https://github.com/toon-format/toon), `toonx` (a TOON variant that also factors out constant fields and splits wide tables) or a strict tabular codec. It switches a route over only after **shadow evaluation on that route's own traffic** shows answers stay as good as with JSON, compared against the model's own noise floor.

> Format effects vary a lot by model and task. Published agentic benchmarks report anywhere from −36pp to +13pp accuracy for TOON. So trimproof never ships an optimization it has not measured on your traffic.

**What we measured** in a pre-registered run on four open models on NVIDIA NIM (gpt-oss-20b, nemotron-3-ultra, glm-5.3-flash and llama-3.2-90b-vision), summarized in the [write-up](docs/writeup.md):
- **The right answer differed by model.** Replaying every answer through trimproof's own promotion test turned `toonx` on for **none** of them:
  - llama and gpt-oss were switched **OFF**: their answers agreed with JSON less often than JSON agreed with itself, confidently by more than the 2pp margin;
  - glm stayed in **SHADOW** after 469 samples, with its bounds straddling the 2pp margin;
  - nemotron-3-ultra stayed in **SHADOW**, and its 7% net saving is below the 15% minimum anyway.
- **Input:** `toonx` used **20–31%** fewer input tokens than compact JSON on every model and data set.
- **Output:** the reasoning models often wrote more on `toonx` (nemotron-3-ultra +69% on synthetic tables). With output priced at 4× input, the net saving ranged from **−4.5% to +30.6%**.
- **Accuracy:** glm and nemotron-3-ultra were non-inferior within 3pp on real API payloads and synthetic tables. gpt-oss lost 7.8pp on synthetic tables, and llama lost 12–14pp on both.
- No Claude or OpenAI-hosted model was tested.

Whether a format pays off depends on the model, the data and the task, so trimproof measures each route instead of assuming.

Full results: [`VERIFY.md`](bench/results/verify-2026-10/VERIFY.md), with every call's record beside it. Pre-registration and amendments: [`VERIFY-PLAN.md`](bench/results/VERIFY-PLAN.md). The earlier two-model study: [`REPORT.md`](bench/results/REPORT.md).

## How it works

```
client ──► ingress (TLS, JWT) ──► trimproof ──► Anthropic / OpenAI-compatible upstream
                                   │ parse → 4 gates → encode → forward → Tier 1 validate
                                   └─► observers: shadow evaluator · promotion · audit · OTel
```

1. **Data-only, request-side transforms.** Only `tool_result` / `role:tool` content that parses as uniform JSON arrays is eligible, plus blocks the client marks with `X-Trimproof-Data`. A route can also opt in to `auto_detect_data`: a user text part that is wholly JSON, or a fenced ```` ```json ```` block inside one, then counts as data, and an encoded fence is relabelled with the codec name. System prompts, tool definitions, assistant turns and tool-call arguments are never touched. Ineligible requests are forwarded **byte-identical**.
2. **Four gates:** structural → opt-in → minimum size → net savings. Net savings are measured against *compact canonical* JSON, and the format primer's token cost is included.
3. **Lossless by construction.** Every codec satisfies `Decode(Encode(x)) == canonical(x)` byte for byte, enforced by fuzz tests. Numbers keep their exact text (`9007199254740993`, `88.0`, `1e400`). toon-go silently loses such numbers, so the `toon` codec admits only payloads that survive its round trip and verifies every encoding.
4. **Promotion state machine per route:** `OFF → SHADOW → ENABLED` (plus `MANUAL`).
   - In `SHADOW`, each sampled request runs three arms: JSON, the codec, and JSON again, so every sample carries its own noise floor. Arms run asynchronously, with their own rate budget.
   - Decisions come only at scheduled looks, through a paired non-inferiority test. A route is promoted when all of these hold: the lower confidence bound of (codec agreement − noise floor) is above −ε; there is no Tier 1 regression (exact McNemar); and the measured savings meet the route's minimum. Savings come from provider `usage`, with output tokens weighted by `output_price_ratio`.
   - A route that is confidently worse is switched `OFF`, which stops its sampling cost.
   - It is demoted automatically on regression, including a production circuit breaker on Tier 1 failure rates.
   - Simulated error rates are in [bench/results/PROMOTION.md](bench/results/PROMOTION.md).
5. **Tier 1 validation (optional, per route).** It is default-deny and all-or-nothing across parallel tool calls:
   - JSON Schema per tool;
   - exact-decimal business rules;
   - scope, tenant and resource authorization against identity from trusted ingress only.

   Failed encoded requests can retry once with canonical JSON. Provider errors never trigger that fallback.
6. **Tier 2 audit.** A bounded queue that drops entries when full. Redaction walks the JSON tree by key. Encoded payloads are logged only as codec + version + SHA-256.

## Install

- **Binaries:** download `trimproof-gateway` and the `trimproof` CLI for Linux, macOS or Windows from [Releases](https://github.com/AkashAgarwalInd/trimproof/releases).
- **From source** (Go 1.27+): `go install github.com/AkashAgarwalInd/trimproof/cmd/trimproof@latest` for the CLI. `go install …/cmd/gateway@latest` installs the gateway as `gateway`.
- **Docker:** build the image from this repository, as below.

## Quick start

Build and run the gateway with the demo policy. The `demo` route is `ENABLED` with the `toon` codec, and identity is not required, so this works without any ingress:

```bash
go build -o bin/gateway ./cmd/gateway
bin/gateway -policies examples/quickstart.json -require-identity=false
```

Or run it with Docker:

```bash
docker build -t trimproof .
docker run -p 8080:8080 trimproof -require-identity=false
```

Send a request through it, once on the `demo` route and once on a route that doesn't exist, which passes through unchanged. The example request asks a question about 40 orders that arrive as a tool result:

```bash
for route in demo none; do
  curl -s -D - -o /dev/null http://localhost:8080/openai/v1/chat/completions \
    -H "Authorization: Bearer $OPENAI_API_KEY" -H 'Content-Type: application/json' \
    -H "X-Trimproof-Route: $route" -d @examples/openai-request.json | grep -i x-trimproof
done
```

On NVIDIA NIM `openai/gpt-oss-20b`:
- the `demo` route reported 1067 prompt tokens instead of 1723 (−38%);
- both answers were the same and correct.

To use another provider, set `-openai-base` (default `$OPENAI_API_BASE` or `https://api.openai.com/v1`) and change `model` in the request.

### Using it from an SDK

Point SDKs at the gateway:
- Anthropic: `base_url = http://localhost:8080/anthropic`
- OpenAI-compatible: `base_url = http://localhost:8080/openai/v1`

Send `X-Trimproof-Route: <route>` on every request. Provider API keys pass through from the client. Responses carry `X-Trimproof-Representation` (`json` or `toon; est_savings=…`).

### In production

- **Identity:** run behind trusted ingress that mints `X-TP-Identity`, an HS256 JWT with claims `tenant_id`, `scope` and `exp`, signed with `TP_IDENTITY_KEY`. Alternatively use `-identity-mode trusted-headers`.
- **Policies:** start new routes in `SHADOW` (see [`examples/policies.json`](examples/policies.json)). The gateway promotes a route on its own once there is enough evidence.
- **State:** promotion state, evaluation pairs and audit logs are JSONL files in the working directory (`/data` in the container). Keep them on a volume.
- **Report:** `trimproof report` reads those files and prints, per route:
  - its state and transitions;
  - the evidence the promoter is deciding on: samples, codec agreement against the noise floor with bounds, net savings and latency;
  - what the next look will do;
  - an audit summary.

  Run it in the gateway's working directory, or pass `-policies`, `-pairs-file`, `-transitions-file` and `-audit-file`. Add `-json` for machine-readable output.
- **Claude token estimates:** the gates count tokens with o200k. For Claude models, the gateway corrects these counts with Anthropic's free `count_tokens` endpoint. At most once per model per `-calibrate-interval` (default 15 minutes), it asks that endpoint for the exact count of one payload as JSON and as the codec. This happens in the background with the client's key. Set `-calibrate-interval 0` to disable this.
- **Metrics:** set `OTEL_EXPORTER_OTLP_ENDPOINT` to export metrics: requests, gate rejections, estimated tokens saved, Tier 1 results, fallbacks and promotion transitions.

## Benchmark (Phase 0)

```bash
go run ./cmd/bench tokens                                   # offline o200k token table
go run ./cmd/bench run -targets openai:MODEL,anthropic:MODEL  # live, resumable
go run ./cmd/bench report -in bench/results/<file>.jsonl
```

## Status

Pre-alpha. Every component has a first implementation with tests: codecs, gateway, Tier 1, shadow evaluation and promotion, audit, telemetry and the report CLI.

Tested end to end against NVIDIA NIM (OpenAI-compatible). Other OpenAI-compatible servers should work through `-openai-base`, but haven't been tested. The Anthropic adapter is covered by tests against a fake upstream only; it has not yet been run against the real API.

Not yet built:
- OpenTelemetry traces (only metrics exist);
- OPA/Cerbos authorizer adapters (the interface exists);
- incremental validation of streamed tool calls (validated streaming routes buffer the full response).

## License

Apache-2.0. A hosted control plane (fleet policy registry, promotion dashboards, cost reports, SSO) is planned as a separate commercial offering.
