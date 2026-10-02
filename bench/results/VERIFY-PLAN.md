# Verification benchmark: pre-registration

**Written:** 2026-10-02, before any verification call was made. The commit that adds this file predates every result it describes.

Phase 0 ([REPORT.md](REPORT.md)) measured 46 questions per model on two models. That is enough to show input savings, but too few to support accuracy claims: its intervals were about ±6–8pp. This run fixes the claims, metrics and decision rules in advance. The published write-up may then state only what these rules allow.

## Questions this run answers

1. **Input tokens:** how much does TOON reduce billed input tokens against compact JSON? By model, overall and by data shape.
2. **Accuracy:** is TOON non-inferior to compact JSON within 3 percentage points? By model, on synthetic and on real tables.
3. **Output tokens:** does TOON change how many output tokens models write, and is that reasoning or answer text? What is left of the savings once output is priced in?
4. **Production path:** do the savings measured through the gateway itself match the direct measurement?
5. **Real tool output:** what share of real public-API responses do the gateway's gates accept, and what do they save?
6. **Tabular codec:** is the Phase 0 tabular signal on nemotron (−8.7pp, not significant) real?

## Cost

Every call goes to free endpoints, and nothing here can be billed:
- **Live calls** go to NVIDIA NIM through a free NVIDIA Developer Program key, which has no payment method.
- **The harness refuses any base URL** outside NIM, GitHub Models, Google's Gemini endpoint or `localhost`, unless `-allow-paid` is passed. This run never passes it.
- **Every live invocation** carries a hard `-max-calls` cap.

## Models (NVIDIA NIM)

| model | family | why |
|---|---|---|
| `openai/gpt-oss-20b` | OpenAI open weights, reasoning | Phase 0 model |
| `nvidia/nemotron-3-super-120b-a12b` | NVIDIA, reasoning | Phase 0 model; tabular signal |
| `mistralai/mistral-large-2-instruct` | Mistral, non-reasoning | separates reasoning effects from format effects |
| `google/gemma-3-12b-it` | Google, small, non-reasoning | small-model sensitivity |
| `deepseek-ai/deepseek-v4.1-flash` | DeepSeek | another family |

**Smoke check and fallbacks.** Each model first gets a 5-call smoke check. A model that fails it (errors, or empty replies on 3 of 5 calls) is replaced:
- gemma-3-12b → `google/gemma-3-4b-it`
- deepseek → `z-ai/glm-5.3-flash`
- any other failure → the model is dropped and the drop is reported

Claude and OpenAI's hosted GPT models are not tested; the write-up must say so.

**Optional, directional only.** These may run on free tiers if the maintainer provides keys. They get no verdicts and are reported separately:
- GitHub Models (GPT family), on 30-row data only;
- Gemini Flash.

## Data

**Synthetic.** The four Phase 0 generators (orders, logs, search, employees) at 30 and 120 rows, with **new seeds 1000–1004**:
- 46 questions per seed, so **230 questions per model**;
- answers are computed from the data.

**Real tables: WikiTableQuestions.** The `pristine-unseen-tables` test split:
- only tables with at least 15 rows;
- **100 questions**, chosen as a fixed sample with seed 1000;
- each table is sent as a JSON array of objects keyed by column header, in the source column order, with cell values as strings;
- scored with a port of the official WTQ evaluator.

## Formats

| set | formats |
|---|---|
| synthetic, all models | `json-compact`, `json-compact-2` (identical resend: the noise floor), `toon` |
| synthetic, nemotron only | also `tabular` |
| WTQ, all models | `json-compact`, `json-compact-2`, `toon` |
| gateway path, gpt-oss, first 120 synthetic questions | `gateway` (JSON sent through a local gateway on an `ENABLED` toon route) vs `json-compact` direct |

Settings: temperature 0 and `max_tokens` 4096, as in Phase 0. An empty reply is retried up to 3 times, then scored incorrect.

## Metrics

All metrics compare a format with `json-compact` on the **same questions** (paired). Only questions where both arms succeeded count.

| metric | definition |
|---|---|
| input savings | 1 − Σ input_tokens(format) / Σ input_tokens(json) |
| output change | Σ output_tokens(format) / Σ output_tokens(json) − 1, split into reasoning and answer tokens where the provider reports reasoning tokens (else estimated from the reasoning text, and marked as estimated) |
| net savings at k | 1 − (Σin_f + k·Σout_f) / (Σin_j + k·Σout_j), for k = 4 (the gateway default `output_price_ratio`) and k = 1 |
| Δacc | accuracy(format) − accuracy(json), over the same questions |
| noise floor | share of questions where `json-compact` and `json-compact-2` disagree on correctness, plus their own Δacc |
| latency ratio | median over questions of latency(format) / latency(json); secondary, because NIM queueing is noisy |

Token counts are the provider's reported `usage`, never estimates.

**Intervals.** Every interval is a 95% percentile bootstrap:
- 2,000 resamples of questions;
- a fixed seed (1);
- computed per model and format.

## Claim rules

**Accuracy**, per model and format, against a margin of 3pp:

| result | wording allowed |
|---|---|
| lower bound of Δacc > −3pp | "non-inferior to JSON within 3pp" |
| upper bound of Δacc < −3pp | "worse than JSON" |
| otherwise | "inconclusive at this sample size" |

**Other rules:**
- Savings are always quoted as the point estimate with its 95% interval, per model. Ranges across models list every model; there is no cherry-picked subset.
- No accuracy claim is made without the noise floor beside it.
- A claim pooled across models is allowed only as "on k of 5 models". There is no pooled significance test.
- WTQ results are reported separately from synthetic ones and never merged.
- **Gateway path:** if the gateway-path savings for gpt-oss fall outside the direct measurement's interval, the write-up reports that discrepancy and its cause.

## Exclusions

- **Failed calls:** HTTP errors and timeouts after the harness's retries are excluded and counted per model and format. A failed call is re-run once, by resuming the run.
- **Persistent failures:** if more than 10% of a model's calls still fail, that model's results are reported but carry no verdict.
- **Questions:** none are removed after seeing results.

## Records of every data point, and of the data itself

Everything measured is kept and published in `bench/results/verify-2026-10/`, so any reader can audit any number.

**Raw records:** one JSONL line per call, including failed calls. Each line holds:
- provider and model;
- data set, seed, row count, question ID, kind;
- the question and the gold answer;
- the format;
- the model's full reply and whether it was scored correct;
- input, output and reasoning tokens;
- latency;
- the gateway's representation header (gateway path only);
- the error, if any.

**Per-data-point reports** (`DATAPOINTS-<set>.md`, generated from the raw records):
- one row per question and model, with every arm side by side: correct or not, input, output and reasoning tokens, latency, and the start of the reply;
- disagreements between arms are marked, so every discordant question can be inspected.

**The data:**
- **Synthetic:** every generated table (as JSON) and its questions with gold answers, written by `bench dump-data` for seeds 1000–1004. This is what the models were sent.
- **WikiTableQuestions:** a manifest of the 100 sampled items. For each item it records:
  - question ID and question;
  - gold answers;
  - source table path, its SHA-256 and row and column counts.

  It also gives the SHA-256 of the release archive. Tables are fetched from the official release, not committed. WikiTableQuestions is by Panupong Pasupat and Percy Liang, licensed CC BY-SA 4.0; questions and answers quoted in the manifest are under that licence.
- **Real API payloads:** a manifest of every fetched URL with fetch time, HTTP status, size, SHA-256 and the gate decision. Third-party payload bodies are not committed.

**Summary report:** `VERIFY.md` is generated from the same records, and states the exact commands that reproduce each table.

## Optimization follow-up (Step 3)

This step runs only if the output increase is confirmed. Rules:
- **Primer variants** (current, one-line minimal, none) are tested on `toon` for gpt-oss and nemotron, 120 synthetic questions each.
- **A variant replaces the shipped primer only if both hold:**
  - its net savings at k=4 are higher than the current primer's, with the bootstrap interval of the difference excluding zero;
  - its Δacc verdict is not worse.
- **Otherwise the primer stays,** and the write-up says the attempt failed.

## Commands

```sh
# offline, no model calls
go run ./cmd/bench payloads -fetch          # public API payloads -> ~/.cache/trimproof/payloads, then the gate report
go run ./cmd/bench fetch-wtq                # WikiTableQuestions release -> ~/.cache/trimproof/wtq (CC BY-SA, not committed)
go run ./cmd/bench dump-data -seed 1000 -seeds 5 -dir bench/results/verify-2026-10/data/synthetic
go run ./cmd/bench dump-data -source wtq -n 100 -seed 1000 -dir bench/results/verify-2026-10/data/wtq

# live: NIM only. The harness refuses non-free endpoints and plans larger than -max-calls.
go run ./cmd/bench run -targets nim:MODEL -seed 1000 -seeds 5 -formats json-compact,json-compact-2,toon \
  -max-calls 690 -rpm 30 -out bench/results/verify-2026-10/synthetic.jsonl
go run ./cmd/bench run -targets nim:nvidia/nemotron-3-super-120b-a12b -seed 1000 -seeds 5 -formats tabular \
  -max-calls 230 -rpm 30 -out bench/results/verify-2026-10/synthetic.jsonl
go run ./cmd/bench run -targets nim:MODEL -source wtq -n 100 -seed 1000 -formats json-compact,json-compact-2,toon \
  -max-calls 300 -rpm 30 -out bench/results/verify-2026-10/wtq.jsonl

# gateway path: a local gateway pinned to NIM, Anthropic unreachable, calibration off
trimproof-gateway -policies verify-policies.json -require-identity=false \
  -openai-base https://integrate.api.nvidia.com/v1 -anthropic-base http://127.0.0.1:9 -calibrate-interval 0
go run ./cmd/bench run -targets nim:openai/gpt-oss-20b -seed 1000 -seeds 5 -n 120 -formats json-compact,gateway \
  -via-gateway http://127.0.0.1:8080/openai/v1 -route verify -max-calls 240 -rpm 30 \
  -out bench/results/verify-2026-10/gateway.jsonl

# reports
go run ./cmd/bench verify-report -in bench/results/verify-2026-10/synthetic.jsonl
go run ./cmd/bench datapoints -in bench/results/verify-2026-10/synthetic.jsonl > bench/results/verify-2026-10/DATAPOINTS-synthetic.md
```

`verify-policies.json` holds one route with the default gates, `{"tenant_id":"*","route_id":"verify","version":"verify.1","codec":"toon","state":"ENABLED","shadow_sample_rate":0,"audit_sample_rate":0}`. The sample rates are 0 so the gateway makes no background evaluation calls: every upstream call is one the harness counted against `-max-calls`.

## Disclosed before any model call

**The real-API payload analysis was not pre-registered.** It ran during harness development, before this file was committed: it makes no model calls and needs no settings beyond the defaults. Its result is reported exactly as produced. Only 3 of 40 public-API responses passed the default gates as sent; the main causes were top-level wrapper objects and rows with optional keys. The write-up must report this.

**Codecs encode the canonical form of each table**, with keys sorted, as the gateway does. JSON arms send each table as the source has it: synthetic tables are already sorted, and WTQ tables keep their column order. TOON columns for a WTQ table can therefore appear in a different order from its JSON; this is what production traffic would see.
