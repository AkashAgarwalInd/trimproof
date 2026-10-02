# Verification benchmark: pre-registration

**Written:** 2026-10-02, before any verification call was made. The commit that adds this file predates every result it describes.

**Amended seven times,** also on 2026-10-02 and before any result: see [Amendment 1](#amendment-1-before-any-result), [Amendment 2](#amendment-2-pilot-before-any-result), [Amendment 3](#amendment-3-free-form-check-before-any-free-form-call), [Amendment 4](#amendment-4-toonx-2-before-any-result), [Amendment 5](#amendment-5-pilot-2-failed-its-rule-diagnostic-before-any-result), [Amendment 6](#amendment-6-split-wide-tables-before-any-result) and [Amendment 7](#amendment-7-row-key-and-marked-prefixes-before-any-result). Where they differ, the later text applies. The original text below is unchanged.

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

## Amendment 1 (before any result)

**Date:** 2026-10-02. This is committed before any verification run.

### Why the plan changed

The payload analysis above found that the gateway encoded only 3 of 40 real API responses, so we fixed the product before measuring it. Plain TOON could not get there, for two reasons:
- **Size.** For nested or irregular rows, TOON's list form is 5–22% *larger* than compact JSON. Allowing wrappers and missing keys still left plain TOON at 3 of 40.
- **toon-go bugs.** toon-go also writes some nested tables in a form its own decoder rejects.

The new codec, `toonx` (`pkg/codec/toonx`), is TOON with three extensions:
- every array of objects becomes a table, and an empty cell means "key absent";
- nested objects may be flattened into `a.b` columns;
- arrays and empty objects in cells are written as JSON.

It has its own encoder and decoder, and every encode is verified by round trip.
- **Plain TOON is unchanged:** on a flat array of uniform objects, toonx writes exactly what TOON writes.
- **Primer:** toonx sends TOON's primer plus one sentence for each extension the request actually uses.

### Coverage of real API responses

Measured offline with no model calls. The reports are [PAYLOADS.md](verify-2026-10/PAYLOADS.md) and [PAYLOADS-HELDOUT.md](verify-2026-10/PAYLOADS-HELDOUT.md).

| set | toonx eligible | toon eligible | savings over all payload tokens, toonx |
|---|---:|---:|---:|
| the 40 payloads toonx was designed against | 23 / 40 | 3 / 40 | 15.4% |
| held-out: 23 new endpoints chosen before measurement, 19 fetched | 8 / 19 | 1 / 19 | 21.4% |

- **Headline:** the held-out set gives the coverage claim, because it was not used for tuning.
- **Frozen sources:** the held-out list (`payloads.HeldOut`) was fixed before it was fetched, and will not be edited.

### Changes to the measurement

1. **Codec under test.** `toonx` replaces `toon` in every arm.
   - Synthetic tables: toonx sends the same bytes and the same primer as TOON on all 40 tables (seeds 1000–1004).
   - WikiTableQuestions: the bytes are identical on 96 of the 100 tables. The other 4 have a dot in a column name, which toonx quotes.
   - The `toon` arm itself is not run.
2. **Primer variant arm.** `toonx-p2` runs on synthetic data for every model. It sends the same encoding as toonx, with this primer in place of TOON's:

   > Some tool results are in TOON: "key[N]{a,b}:" is a table of N rows with fields a and b, one comma-separated row per line; "key[N]: x,y" is a list of values; "key: value" is a field and nested objects are indented. Read the data as given; there is no need to convert it.

   **Adoption rule:** toonx-p2's primer replaces TOON's in toonx only if both hold:
   - against toonx, the cost at k=4 is lower with the 95% interval excluding 0, on at least 3 of the 5 models;
   - no model's accuracy verdict against JSON is worse for toonx-p2 than for toonx.

   `verify-report` applies this rule in its "Primer variant" section. This replaces the separate Step 3 experiment, so no second run is needed.
3. **New data set: payload Q&A.**
   - **Size:** 104 questions over 26 real API responses.
     - Sources: the tuning and held-out payloads that toonx's default gates encode, after cutting.
     - Cutting: each payload's largest array of objects is cut to its first 20 rows, halved until the payload is at most 16,000 o200k tokens, and never below 5 rows.
   - **Questions:** up to 4 per payload, generated with seed 1000. Gold answers are computed from the data. The kinds are:
     - lookup of a nested field (27);
     - lookup of a field that some rows lack or hold as null (18; the gold answer "none" in 12 of them);
     - count of rows with a value (21);
     - the row with the largest number (19);
     - plain lookups (19).
   - **Scoring:** with the WikiTableQuestions evaluator.
   - **Formats:** `json-compact`, `json-compact-2` and `toonx`.
   - **JSON arms** send the cut payload as canonical compact JSON (keys sorted).
   - **Records:** [data/payload-qa/manifest.json](verify-2026-10/data/payload-qa/manifest.json) lists every source URL, response SHA-256, rows kept, question and gold answer. Response bodies are third-party content and are not committed.
   - **Reporting:** results are reported separately, with the same claim rules as WikiTableQuestions, and never merged with other sets.
4. **Gateway path.** The verify route uses `"codec":"toonx"`.
5. **Models.** On 2026-10-02 the interrupted smoke check sent 20 free NIM calls (`json-compact` only), recorded in [smoke.jsonl](verify-2026-10/smoke.jsonl):
   - `mistralai/mistral-large-2-instruct` and `google/gemma-3-12b-it` returned HTTP 404 on every call;
   - gpt-oss-20b and nemotron-3-super answered;
   - deepseek had not been reached.

   These calls test availability only and enter no result. Each slot now has an ordered list. The first model that answers at least 3 of 5 smoke calls is used, and the substitution is reported:

   | slot | candidates, in order |
   |---|---|
   | OpenAI open weights, reasoning | `openai/gpt-oss-20b` |
   | NVIDIA, reasoning | `nvidia/nemotron-3-super-120b-a12b` |
   | Mistral, non-reasoning | `mistralai/mistral-large-2-instruct`, `mistralai/mistral-large`, `nv-mistralai/mistral-nemo-12b-instruct` |
   | Google, non-reasoning | `google/gemma-3-12b-it`, `google/gemma-4-31b-it`, `google/gemma-3-4b-it` |
   | DeepSeek | `deepseek-ai/deepseek-v4.1-flash`, `z-ai/glm-5.3-flash` |

### Calls

| run | calls |
|---|---:|
| synthetic: 230 questions × (`json-compact`, `json-compact-2`, `toonx`, `toonx-p2`) × 5 models | 4,600 |
| synthetic: `tabular` on nemotron | 230 |
| WikiTableQuestions: 100 × 3 formats × 5 models | 1,500 |
| payload Q&A: 104 × 3 formats × 5 models | 1,560 |
| gateway path: 120 × 2 formats, gpt-oss | 240 |
| **total** | **8,130** |

Smoke checks add 5 calls per candidate tried. All calls go to NIM's free tier.

### Commands that change

```sh
go run ./cmd/bench payloads -fetch -held-out      # held-out payloads -> ~/.cache/trimproof/payloads-heldout
go run ./cmd/bench run -targets nim:MODEL -seed 1000 -seeds 5 -formats json-compact,json-compact-2,toonx,toonx-p2 \
  -max-calls 920 -rpm 30 -out bench/results/verify-2026-10/synthetic.jsonl
go run ./cmd/bench run -targets nim:MODEL -source wtq -n 100 -seed 1000 -formats json-compact,json-compact-2,toonx \
  -max-calls 300 -rpm 30 -out bench/results/verify-2026-10/wtq.jsonl
go run ./cmd/bench run -targets nim:MODEL -source payloads -seed 1000 -formats json-compact,json-compact-2,toonx \
  -max-calls 312 -rpm 30 -out bench/results/verify-2026-10/payloads.jsonl
```

The verify route becomes `{"tenant_id":"*","route_id":"verify","version":"verify.2","codec":"toonx","state":"ENABLED","shadow_sample_rate":0,"audit_sample_rate":0}`.

## Amendment 2: pilot (before any result)

Before the full run, a pilot of about 100 free NIM calls checks that the run works end to end. Its questions are disjoint from every registered question, and it enters no result.

**What it checks:**
- each model slot answers (the 5-call smoke check, with the fallbacks of Amendment 1);
- replies parse and are scored, with no errors, empty replies or truncation;
- models can answer from the toonx forms on real payloads: nested paths, absent cells and JSON cells;
- the gateway path serves toonx and records it.

**Data:** synthetic seed 2000, payload Q&A seed 2001 and WikiTableQuestions seed 2002. Questions are drawn with `-sample`, which takes one question of each kind in turn. These seeds were chosen as the first ones whose sampled questions do not appear in the registered sets.

| run | calls |
|---|---:|
| smoke: `mistralai/mistral-large`, `google/gemma-4-31b-it`, `deepseek-ai/deepseek-v4.1-flash` (gpt-oss and nemotron passed on 2026-10-02) | 15, +5 per fallback |
| payload Q&A: 5 questions × (`json-compact`, `toonx`) × 5 models | 50 |
| synthetic: 3 questions × 4 formats, gpt-oss | 12 |
| WikiTableQuestions: 3 questions × (`json-compact`, `toonx`), gpt-oss | 6 |
| gateway path: 4 questions × (`json-compact`, `gateway`), gpt-oss | 8 |
| **total** | **91, at most 101** |

**Allowed after the pilot without a new amendment:**
- harness bug fixes;
- model substitutions under the smoke rule.

Any change to a codec, primer, format or question set would need Amendment 3, committed before the full run, with the pilot numbers that motivated it. Pilot records go to [pilot.jsonl](verify-2026-10/pilot.jsonl), and smoke records to smoke.jsonl.

```sh
go run ./cmd/bench run -targets nim:MODEL -seed 1000 -n 5 -formats json-compact -max-calls 5 -out bench/results/verify-2026-10/smoke.jsonl
go run ./cmd/bench run -targets nim:M1,...,nim:M5 -source payloads -seed 2001 -sample 5 -formats json-compact,toonx -max-calls 50 -out bench/results/verify-2026-10/pilot.jsonl
go run ./cmd/bench run -targets nim:openai/gpt-oss-20b -seed 2000 -sample 3 -formats json-compact,json-compact-2,toonx,toonx-p2 -max-calls 12 -out bench/results/verify-2026-10/pilot.jsonl
go run ./cmd/bench run -targets nim:openai/gpt-oss-20b -source wtq -n 100 -seed 2002 -sample 3 -formats json-compact,toonx -max-calls 6 -out bench/results/verify-2026-10/pilot.jsonl
go run ./cmd/bench run -targets nim:openai/gpt-oss-20b -seed 2000 -sample 4 -formats json-compact,gateway -via-gateway http://127.0.0.1:8080/openai/v1 -route verify -max-calls 8 -out bench/results/verify-2026-10/pilot.jsonl
```

## Amendment 3: free-form check (before any free-form call)

**Date:** 2026-10-02. This is committed before any free-form call. The pilot motivated nothing here: it changes no codec, primer, format or registered question.

**Why:** every registered question has a short gold answer. Real requests also ask for summaries, comparisons and descriptions, where a reply can be fluent and still wrong. This check asks whether toonx changes those answers. It is a separate, clearly labelled check: it enters none of the verdicts above.

**Tasks:** one open task per payload in the payload Q&A set (26 payloads, the same cut data), with no gold answer. Payloads are shuffled with seed 3000 and take these kinds in turn:
- **summary:** 3 to 5 bullet points naming items and values;
- **highlights:** the 3 items that stand out most, with their values;
- **describe:** one item, chosen by its ID, in a short paragraph;
- **compare:** two items, chosen by ID, listing the fields where they differ.

The task list is in the dumped data (`bench dump-data -source freeform -seed 3000`).

**Arms and models:**
- Arms: `json-compact`, `json-compact-2` (the noise floor) and `toonx`.
- Models: the three that passed the smoke rule (`openai/gpt-oss-20b`, `nvidia/nemotron-3-super-120b-a12b`, `z-ai/glm-5.3-flash`).
- Settings: max_tokens 4096 and temperature 0, with empty replies retried as registered.
- System prompt: a free-form one ("Be accurate and concise. Do not mention the format the data came in."). The last sentence keeps the judge blind; it is the same for every arm.

**Judge:** one model from a family not under test grades every answer.
- It sees the data as compact JSON, the request, and the three answers under labels A, B and C, shuffled per task with seed 3000.
- For each answer it lists the statements the data contradicts or does not support, and rates completeness from 1 to 5. For each pair it says whether the two state the same facts.
- Its settings are max_tokens 8192 and temperature 0. A reply that is not valid JSON is retried up to twice more, then excluded and counted.
- **Choice:** the first of `moonshotai/kimi-k3`, `moonshotai/kimi-k2.6` and `nvidia/llama-3.1-nemotron-ultra-253b-v1` that returns valid grades on 2 of 2 smoke calls. Those calls grade pilot answers, not free-form ones. The last candidate shares a family with nemotron; if it is used, the write-up says so.

**Metrics,** per model and over all models, with 95% bootstrap intervals over tasks:
- share of answers with at least one judged error, per arm, and the differences toonx − json and json-2 − json;
- mean completeness per arm;
- agreement: the judge's "same facts" rate and the word-overlap F1, for json vs toonx against json vs json-2;
- input tokens saved and output tokens per arm.

**Claim rule:**
- The write-up may say free-form answers showed **no large quality drop** only if, over all models, the 95% upper bound of toonx − json in the share of answers with an error is at most 15pp.
- Otherwise it reports the gap as measured.
- With about 78 paired answers this check cannot show non-inferiority, and the write-up never claims it. Per-model rows are descriptive.
- Judge errors are the judge's opinion; the write-up says so and links every judged record.

**Calls:**

| run | calls |
|---|---:|
| judge smoke check: 2 per candidate tried | 2–6 |
| answers: 26 tasks × 3 arms × 3 models | 234 |
| judge: 26 tasks × 3 models | 78 |
| **total** | **314–318**, plus registered retries |

Records go to `freeform.jsonl` (answers), `freeform-judged.jsonl` and `judge-smoke.jsonl`, all in `verify-2026-10/`.

```sh
go run ./cmd/bench run -targets nim:openai/gpt-oss-20b,nim:nvidia/nemotron-3-super-120b-a12b,nim:z-ai/glm-5.3-flash -source freeform -seed 3000 -formats json-compact,json-compact-2,toonx -max-calls 234 -out bench/results/verify-2026-10/freeform.jsonl
go run ./cmd/bench judge -source freeform -seed 3000 -in bench/results/verify-2026-10/freeform.jsonl -judge nim:JUDGE -max-tokens 8192 -max-calls 78 -judged bench/results/verify-2026-10/freeform-judged.jsonl
go run ./cmd/bench freeform-report -in bench/results/verify-2026-10/freeform.jsonl -judged bench/results/verify-2026-10/freeform-judged.jsonl
```

## Amendment 4: toonx 2 (before any result)

**Date:** 2026-10-02. This is committed before the pilot it describes and before any registered run.

### What changes

toonx becomes version 2, with two more lossless forms. Each applies only to tables of at least 3 rows, and only when it makes the table shorter:
- **Constant fields.** A field with the same value in every row is written once, on an `all rows: a=x` line under the header, instead of in each row.
- **URL prefixes.** In a column of strings that share an `http(s)://` start, the start (cut after its last `/`) is written once, on a `starts with: a=p` line, and left out of the column's strings. Dates, names and other text are never factored.

Every encode is still decoded and compared with the original, byte for byte. Each form adds one primer sentence, sent only when a request uses it:

> A line "all rows: a=x" under a table header means every row also has field a with value x.
>
> A line "starts with: a=p" under a table header means every text value of field a starts with p, which is left out of the rows: put p back in front.

### Why

The pilot (Amendment 2) measured 20.7% fewer input tokens for toonx over 21 pairs, below the 30–40% hoped for. Its payloads were mostly API objects that repeat the same values and URL starts in every row. In the GitHub example, 13 `*_url` columns share prefixes, and `site_admin` and `user_view_type` are constant.

### Measured offline (no model calls), toonx 1 → toonx 2

| data | toonx 1 | toonx 2 |
|---|---:|---:|
| tuning payloads (40): token-weighted net savings, all payloads | 15.4% | 34.4% |
| tuning payloads: encoded by the default gates | 23 / 40 | 30 / 40 |
| held-out payloads (19): token-weighted net savings, all payloads | 21.4% | 35.8% |
| held-out payloads: encoded by the default gates | 8 / 19 | 11 / 19 |
| payload Q&A, the same 26 cut payloads, primer included | 24.9% | 39.4% |
| synthetic, 40 tables | 26.1% | 26.1% (same bytes) |
| WikiTableQuestions, 100 tables | 34.3% | 34.4% (8 tables factored) |

- **Tuning:** the forms were designed while looking at the tuning payloads. The held-out set, fetched before this change, shows a similar gain.
- **Records:** [PAYLOADS.md](verify-2026-10/PAYLOADS.md) and [PAYLOADS-HELDOUT.md](verify-2026-10/PAYLOADS-HELDOUT.md) are regenerated with toonx 2. The toonx 1 versions stay in git history.

### Effect on the registered sets

- **Synthetic:** no change; toonx 2 sends the same bytes as TOON on all 40 tables.
- **WikiTableQuestions:** 8 of the 100 tables are now factored. Questions are unchanged.
- **Payload Q&A:** the default gates now encode 36 cut payloads instead of 26, so the set grows from 104 to 139 questions (seed 1000).
  - The 26 earlier payloads keep exactly the same questions.
  - The 10 new payloads get questions by the same rules: github `torvalds-events`, `go-issues`, `rust-closed-issues`, `linux-commits` and `node-tags`; `rickandmorty`; `itunes`; coingecko `exchanges`; `pokeapi`; `federalregister`.
  - Calls: 139 × 3 formats × 5 models = 2,085 (was 1,560).
  - The manifest in `data/payload-qa/` is regenerated.
- **Free-form check (Amendment 3):** it now has 36 tasks; the kinds are assigned in turn after the same seeded shuffle.
  - Calls: 36 × 3 arms × 3 models = 324 answers, plus 108 judge calls.
  - It had made 20 answer calls with toonx 1 when it was stopped for this change. Those records are kept in [freeform-stopped.jsonl](verify-2026-10/freeform-stopped.jsonl) and enter no result; the check restarts from zero.
  - The judge smoke check (2 calls, kimi-k3 passed) stands; it is recorded in [judge-smoke.jsonl](verify-2026-10/judge-smoke.jsonl).

### Pilot 2: accuracy on the new forms

**Questions:** 6 payload Q&A questions drawn with seed 2004. They were chosen before any call because they touch the new forms. None appears in the registered set or in pilot 1.

| question | what it tests |
|---|---|
| `payload-github-react-contributors-q2` | rebuild a URL from its prefix (`followers_url`) |
| `payload-tvmaze-shows-q1` | rebuild a nested URL from its prefix (`_links.self.href`) |
| `payload-dockerhub-library-repos-q4` | read a field from the `all rows` line (`namespace`) |
| `payload-github-rust-closed-issues-q1` | read a constant nested URL (`milestone.creator.repos_url`) |
| `payload-gitlab-projects-q3` | count rows in a factored table |
| `payload-github-vscode-releases-q3` | largest value in a factored table |

**Run:** `json-compact` against `toonx` on gpt-oss-20b, nemotron-3-super and glm-5.3-flash: 36 calls.

**Decision rule, fixed now:**
- **toonx 2 is kept** if, over the 18 pairs, toonx answers correctly at most 1 time fewer than JSON.
- **Otherwise:** for each form, if toonx misses where JSON is right on 2 or more of that form's pairs, the form is removed before the full run. The URL form's pairs are the first two questions; the constants form's are the third and fourth. Questions five and six count against both.
- The outcome is written up in [PILOT.md](verify-2026-10/PILOT.md) before the full run.

```sh
go run ./cmd/bench run -targets nim:openai/gpt-oss-20b,nim:nvidia/nemotron-3-super-120b-a12b,nim:z-ai/glm-5.3-flash \
  -source payloads -seed 2004 -only payload-github-react-contributors-q2,payload-tvmaze-shows-q1,payload-dockerhub-library-repos-q4,payload-github-rust-closed-issues-q1,payload-gitlab-projects-q3,payload-github-vscode-releases-q3 \
  -formats json-compact,toonx -max-calls 36 -out bench/results/verify-2026-10/pilot2.jsonl
```

The full-run commands are unchanged except for these call caps: payload Q&A `-max-calls 417` per model, and free-form answers `-max-calls 324` with judge `-max-calls 108`.

## Amendment 5: pilot 2 failed its rule; diagnostic (before any result)

**Date:** 2026-10-02, after pilot 2 and before the diagnostic below.

**Pilot 2 outcome** ([pilot2.jsonl](verify-2026-10/pilot2.jsonl)): JSON 17/18 correct, toonx 2 15/18. That is 2 fewer, against an allowed 1.
- Both misses are nemotron-3-super on toonx, on `gitlab-projects-q3` (count) and `github-vscode-releases-q3` (largest value). Each reasoned for all 4,096 output tokens and gave no answer.
- Both count against both forms, so **under Amendment 4's rule both forms are removed.**
- Every lookup that needed a form was answered correctly by every model (12 of 12).
- Pilot 1 had the same failure with toonx 1, which has neither form: nemotron, `tvmaze-shows-q4` (largest value), 4,096 tokens, no answer.

The rule cannot tell "the forms hurt" from "nemotron runs out of output tokens on toonx". This diagnostic tests which, and how its outcome will be read is fixed here.

**Diagnostic:**
- **Model:** nemotron-3-super only.
- **Questions:** the three that failed: `payload-gitlab-projects-q3` and `payload-github-vscode-releases-q3` (seed 2004), and `payload-tvmaze-shows-q4` (seed 2001).
- **Arms:** `toonx1` is toonx with the version 2 forms turned off; its bytes and primer equal toonx 1's on all 62 payloads. Each call runs 3 times.

| limit | arms | calls |
|---|---|---:|
| 4,096 output tokens | `json-compact`, `toonx1`, `toonx` | 27 |
| 16,384 output tokens | `toonx1`, `toonx` | 18 |
| **total** | | **45** |

**Reading, fixed now** (9 calls per arm and limit):
- **The forms are the cause** if, at 4,096, `toonx` fails at least 3 more times than `toonx1`. Then Amendment 4's rule stands and both forms are removed.
- **Otherwise the forms are not the cause.** toonx 2 is kept, and the write-up says that pilot 2's rule fired, and why it was set aside, linking this amendment.
- **The output limit is the cause** if, at 16,384, `toonx1` and `toonx` each answer correctly at least 8 of 9 times. Then the full run uses max_tokens 16,384 for every model and arm, so no arm is cut off; output tokens are still counted in full.
- **Otherwise** max_tokens stays 4,096, and toonx's extra output on nemotron is reported as a finding.

Records go to `verify-2026-10/diag/`, one file per repeat and limit.

**Outcome** ([PILOT.md](verify-2026-10/PILOT.md#pilot-2-and-its-diagnostic-2026-10-02)): toonx 1 failed as often as toonx 2 at 4,096 (8 and 9 of 9 cut off), and both answered 9 of 9 at 16,384. So **toonx 2 is kept**, and **every full-run command uses `-max-tokens 16384`**. The per-call timeout in the harness is now 12 minutes.

## Amendment 6: split wide tables (before any result)

**Date:** 2026-10-02, after Amendment 5's outcome and before any call with the changes below.

**Why.** Pilot 2 counted input and output tokens equally. With output priced at 4× input (the gateway's default `output_price_ratio`), toonx's saving against compact JSON falls a long way:

| model | input saved | cost saved, output at 4× |
|---|---:|---:|
| gpt-oss-20b | 38.8% | 19% |
| glm-5.3-flash | 38.4% | 29% |
| nemotron-3-super | 35.8% | −17% |

**Diagnosis.** 10 further free NIM calls captured the reasoning text: 5 questions × `json-compact` and `toonx`, at 16,384 output tokens. The full responses are in [reasoning/](verify-2026-10/reasoning/). These are not results.
- **What the models do:** on toonx, every model lists the header, numbers the columns and counts commas row by row to find a value.
- **Why that costs so much:** the tables are 26–42 columns wide, so this takes thousands of tokens, and nemotron used all 16,384 once.
- **Two misreads by gpt-oss:**
  - one column off on a 39-column table;
  - a URL rest written as `"266"` taken for the `id` column.
- **Width matters in Phase 0 too.** Output tokens rose most on its widest table (employees, about 13 columns).
- **Real APIs are wide.** Only 7 of the 36 Q&A payloads have at most 10 varying columns.

**Change 1: toonx's URL prefix (all toonx arms).**
- **Rule:** a "starts with" prefix is now cut one `/` earlier when a value's rest would be only digits. So `https://api.tvmaze.com/shows/266` becomes `shows/266` under `https://api.tvmaze.com/`, not `"266"`.
- **Effect:** coverage figures in [PAYLOADS.md](verify-2026-10/PAYLOADS.md) and [PAYLOADS-HELDOUT.md](verify-2026-10/PAYLOADS-HELDOUT.md) fall by at most 0.6pp; both files are regenerated.
- **The payload Q&A set is unchanged:** `data/payload-qa/manifest.json` is byte for byte the same under the registered seed.

**Change 2: a new arm, `toonx-split`.**
- **The split:** toonx, with every array of objects that has more than 8 varying columns cut into tables `part1`, `part2`, … of at most 8 columns.
- **The row column:** each table starts with a row-number column `#`, and all hold the same rows in the same order.
- **Grouping:** columns are grouped by their first key, with top-level fields first. Fields constant in every row stay with `part1`.
- **Primer:** one sentence is added, only when a table was split.
- **Code:** for now it is a benchmark renderer (`internal/bench/split.go`). A test checks that merging the parts gives back the input exactly.

**Offline gate, met before any call** ([SPLIT-TOKENS.md](verify-2026-10/SPLIT-TOKENS.md)). The plan was to continue only if `toonx-split` keeps at least 25% of the input tokens.

| format | input tokens vs compact JSON, 36 payloads |
|---|---:|
| toonx | −33.7% |
| `toonx-split` | **−28.2%** |
| toonx only on tables of ≤8 columns, else JSON | −1.7% |

**Pilot 3.**
- **Models:** the same 3 models.
- **Questions:** 10 payload Q&A questions, seed 2004:
  - pilot 2's 6: `payload-dockerhub-library-repos-q4`, `payload-github-react-contributors-q2`, `payload-github-rust-closed-issues-q1`, `payload-github-vscode-releases-q3`, `payload-gitlab-projects-q3`, `payload-tvmaze-shows-q1`;
  - 4 aggregations on wide tables: `payload-github-google-repos-q4`, `payload-github-search-repos-q2`, `payload-tvmaze-schedule-us-q3`, `payload-github-linux-commits-q3`.
- **Arms:** `json-compact`, `toonx` and `toonx-split`, at `-max-tokens 16384`.
- **Calls:** 90, to `verify-2026-10/pilot3.jsonl`.
- **Cost of an arm** for a model: input + 4 × output tokens, summed over the questions where every arm returned a record.

**Reading, fixed now:**
- **A candidate qualifies** if both hold:
  - its pooled cost saving against `json-compact`, over all 3 models, is at least 15%;
  - on every model it answers at most 1 question fewer correctly than `json-compact`.
- **If both candidates qualify,** the one with the larger pooled cost saving is chosen. If only one qualifies, it is chosen.
  - **If that is `toonx-split`,** it is built into the toonx codec as version 3, with a decoder and round-trip tests, and with byte-identical output to this renderer on the 36 payloads. The full run's toonx arm then uses it.
- **If neither qualifies,** the full run's toonx arm uses toonx only when no table is wider than 8 varying columns. The write-up must then say that compression is applied to narrow tables only.
- **Every model's cost saving is reported for both candidates,** including any that is negative.

**Outcome** ([PILOT.md](verify-2026-10/PILOT.md#pilot-3-split-wide-tables-2026-10-02)):
- **`toonx-split` qualifies:** 35.0% pooled cost saving. Correct answers: gpt-oss 8, nemotron 10, glm 10, against JSON's 9, 9 and 10.
- **toonx does not qualify:** −2.2%.
- **Next:** `toonx-split` is built into the toonx codec as version 3, and the full run's toonx arm uses it.

**Built as toonx version 3,** before any further call:
- **Output check:** on all 36 payloads, the codec's bytes and primer are identical to pilot 3's `toonx-split` arm.
- **Format names:** `toonx` now sends version 3, and `toonx-split` is an alias for it. Version 2 is still available as `toonx2`, identical to pilot 3's toonx arm on all 36 payloads.
- **The payload Q&A set stays as registered.** It was chosen by toonx 2's gates, and still is: the selection now names `toonx2`. Under the registered seed, the only change to `data/payload-qa/manifest.json` is its `codec` label.
  - Under toonx 3's gates, 8 of the 36 would fall below the 15% savings gate, because splitting costs some input tokens.
  - The full run keeps all 36 payloads. For those 8, the write-up must say that the gateway would have sent JSON.
- **Coverage reports regenerated for toonx 3:**

  | set | eligible payloads (toonx 2 → 3) | input saved over all payloads (toonx 2 → 3) |
  |---|---|---|
  | tuning ([PAYLOADS.md](verify-2026-10/PAYLOADS.md)) | 30/40 → 27/40 | 34.2% → 30.2% |
  | held-out ([PAYLOADS-HELDOUT.md](verify-2026-10/PAYLOADS-HELDOUT.md)) | 11/19 → 8/19 | 35.7% → 32.5% |

## Amendment 7: row key and marked prefixes (before any result)

**Date:** 2026-10-02, after pilot 3 and the outlier check in [PILOT.md](verify-2026-10/PILOT.md#outlier-check-on-toonx-3-2026-10-02), before any call with the changes below.

**Why.** The outlier check found two costs in toonx 3:
- **Lookups need a join across parts.** The field used to find a row (usually `id`) and the field asked for are often in different parts. Models then find the row number in one part and read the other part, and nemotron writes out every row while doing it.
- **"starts with" prefixes are left off.** gpt-oss answered `shows/266` without its prefix, twice.

**Change 1: row key (toonx 4).**
- **The key column:** each part of a split table starts with the row's key field k, as column `#k`, in place of the row number. The field itself is not repeated elsewhere.
- **Choosing the key:** a top-level field that every row has, a string or number, unique across rows. Preference order: `id`, then names ending in `id`, `key`, then `name`.
- **No key:** the parts start with the row number `#`, as in toonx 3.
- **The rule was written without looking at the questions.** It names the question's lookup field in 66 of the 87 lookups in the registered set. The other 21 lookups still need a join.

**Change 2: marked prefixes (toonx 4).**
- **The mark:** in a column with a "starts with" line, each value is written as `~` followed by its rest.
- **The primer** now says that `~x` means the prefix followed by x.

**Earlier versions stay available, byte for byte:**
- **`toonx3`** reproduces pilot 3's `toonx-split` arm on all 36 payloads; `toonx-split` is now an alias for it.
- **`toonx2`** reproduces version 2; it still selects the registered Q&A set, and `data/payload-qa/manifest.json` is unchanged.

**Offline cost** ([TOONX4-TOKENS.md](verify-2026-10/TOONX4-TOKENS.md)), input tokens against compact JSON on the 36 payloads:

| format | input saved |
|---|---:|
| toonx 2 | 33.7% |
| toonx 3 | 28.2% |
| toonx 4 | 23.6% |

The coverage reports are regenerated for toonx 4:

| set | eligible payloads | input saved over all payloads |
|---|---|---|
| tuning | 27/40 → 24/40 | 30.2% → 27.0% |
| held-out | 8/19 → 8/19 | 32.5% → 28.0% |

**Pilot 4.**
- **Questions:** pilot 3's 10 questions (seed 2004).
- **Models:** the same 3 models.
- **Arms:** `json-compact`, `toonx3` and `toonx` (version 4), at `-max-tokens 16384`.
- **Repeats:** each call is made twice, to `verify-2026-10/pilot4-r1.jsonl` and `pilot4-r2.jsonl`. That is 180 calls.
- **Cost** as in Amendment 6: input + 4 × output tokens, summed over both repeats and the questions where every arm returned a record.

**Reading, fixed now:**
- **A candidate (`toonx3` or `toonx` version 4) qualifies** if both hold:
  - its pooled cost saving against `json-compact` is at least 15%;
  - on every model it answers at most 2 fewer correctly than `json-compact`, out of 20.
- **Choosing:** of the qualifying candidates, the one with the larger pooled cost saving is used in the full run.
- **If neither qualifies,** the full run's toonx arm uses toonx only on tables of at most 8 varying columns, and the claims narrow as in Amendment 6.
- **Always reported, whatever the outcome:**
  - every model's cost saving;
  - output tokens on lookups;
  - the number of answers that leave out a "starts with" prefix.

**Outcome** ([PILOT.md](verify-2026-10/PILOT.md#pilot-4-toonx-4-against-toonx-3-2026-10-0203)):
- **Both versions qualify.** toonx 3 saves 30.1% cost pooled and toonx 4 21.6%.
- **toonx 3 is chosen.** toonx 4 is not adopted.
- **The codec's default is version 3 again,** byte for byte as in pilot 3. Version 4 remains available as the `toonx4` bench format.
- **The full run's toonx arm uses toonx 3.**
