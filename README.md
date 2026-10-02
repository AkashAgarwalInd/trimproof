# trimproof

[![ci](https://github.com/AkashAgarwalInd/trimproof/actions/workflows/ci.yml/badge.svg)](https://github.com/AkashAgarwalInd/trimproof/actions/workflows/ci.yml)

**The measured, lossless context optimizer for LLM traffic.**

trimproof is a Go gateway for the Anthropic Messages and OpenAI-compatible Chat Completions APIs. It re-encodes tabular tool results (DB rows, logs, search hits) into token-efficient formats such as [TOON](https://github.com/toon-format/toon) or a strict tabular codec. It switches a route over only after **shadow evaluation on that route's own traffic** shows answers stay as good as with JSON, compared against the model's own noise floor.

> Format effects vary a lot by model and task. Published agentic benchmarks report anywhere from −36pp to +13pp accuracy for TOON. So trimproof never ships an optimization it has not measured on your traffic.

Phase 0 benchmark: [`bench/results/REPORT.md`](bench/results/REPORT.md).

## How it works

```
client ──► ingress (TLS, JWT) ──► trimproof ──► Anthropic / OpenAI-compatible upstream
                                   │ parse → 4 gates → encode → forward → Tier 1 validate
                                   └─► observers: shadow evaluator · promotion · audit · OTel
```

1. **Data-only, request-side transforms.** Only `tool_result` / `role:tool` content (and blocks the client marks with `X-Trimproof-Data`) that parse as uniform JSON arrays are eligible. System prompts, tool definitions, assistant turns and tool-call arguments are never touched. Ineligible requests are forwarded **byte-identical**.
2. **Four gates:** structural → opt-in → minimum size → net savings. Net savings are measured against *compact canonical* JSON, and the format primer's token cost is included.
3. **Lossless by construction.** Every codec satisfies `Decode(Encode(x)) == canonical(x)` byte for byte, enforced by fuzz tests. Numbers keep their exact text (`9007199254740993`, `88.0`, `1e400`). toon-go silently loses such numbers, so the `toon` codec admits only payloads that survive its round trip and verifies every encoding.
4. **Promotion state machine per route:** `OFF → SHADOW → ENABLED` (plus `MANUAL`).
   - In `SHADOW`, sampled requests run as JSON-vs-codec *treatment* pairs and JSON-vs-JSON *control* pairs, asynchronously and with their own rate budget.
   - A route is promoted only when all of these hold: enough pairs; agreement within ε of the noise floor; no Tier 1 regression (exact McNemar); and measured savings (from provider `usage`) at or above the route's minimum.
   - It is demoted automatically on regression, including a production circuit breaker on Tier 1 failure rates.
5. **Tier 1 validation (optional, per route).** It is default-deny and all-or-nothing across parallel tool calls:
   - JSON Schema per tool;
   - exact-decimal business rules;
   - scope, tenant and resource authorization against identity from trusted ingress only.

   Failed encoded requests can retry once with canonical JSON. Provider errors never trigger that fallback.
6. **Tier 2 audit.** A bounded queue that drops entries when full. Redaction walks the JSON tree by key. Encoded payloads are logged only as codec + version + SHA-256.

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
- **Metrics:** set `OTEL_EXPORTER_OTLP_ENDPOINT` to export metrics: requests, gate rejections, estimated tokens saved, Tier 1 results, fallbacks and promotion transitions.

## Benchmark (Phase 0)

```bash
go run ./cmd/bench tokens                                   # offline o200k token table
go run ./cmd/bench run -targets openai:MODEL,anthropic:MODEL  # live, resumable
go run ./cmd/bench report -in bench/results/<file>.jsonl
```

## Status

Pre-alpha. All phases of the spec have a first implementation with tests: core, gateway, Tier 1, shadow evaluation and promotion, audit and telemetry. Not yet built:
- OpenTelemetry traces (only metrics exist);
- OPA/Cerbos authorizer adapters (the interface exists);
- incremental validation of streamed tool calls (validated streaming routes buffer the full response).

## License

Apache-2.0. A hosted control plane (fleet policy registry, promotion dashboards, cost reports, SSO) is planned as a separate commercial offering.
