# Verification run: results

Run on 2026-10-03 and 2026-10-04 under [VERIFY-PLAN.md](../VERIFY-PLAN.md): Amendment 8 (the full run), Amendment 9 (a model retired mid-run) and one deviation (llama's WikiTableQuestions stage stopped early). Every table below is generated from the committed records by the command above it. Per-question records: `DATAPOINTS-<stage>.md` and the `.jsonl` files beside this one.

## Result

**trimproof's promotion test would not have turned toonx on for any model tested.** The test was replayed on every question that all three arms answered (payload Q&A, synthetic and WikiTableQuestions, pooled per model):

| model | decision | why |
|---|---|---|
| llama-3.2-90b-vision (non-reasoning) | **OFF** at 200 samples | agreement with JSON 0.623, against a JSON-vs-JSON noise floor of 0.965 |
| gpt-oss-20b | **OFF** at 400 samples | agreement 0.856 against 0.922 |
| glm-5.3-flash | **SHADOW**, undecided after 469 samples | the difference bounds [−0.044, −0.001] straddle the −0.02 margin; net saving 21.8% |
| nemotron-3-ultra | **SHADOW**, undecided after 469 samples | bounds straddle the margin, and its net saving (7.1%) is below the 15% minimum, so it could not be promoted |
| nemotron-3-super (partial: retired mid-run) | SHADOW | 76 samples, below the first look at 200 |

**Input tokens:** toonx used 20–31% fewer input tokens than compact JSON on every model and every set, with intervals within about ±5pp.

**Net saving, output priced at 4× input** (each with its 95% interval in the tables below):

| model | payload Q&A | synthetic | WikiTableQuestions |
|---|---:|---:|---:|
| glm-5.3-flash | +27.1% | +17.3% | +25.6% |
| llama-3.2-90b-vision | +30.3% | +29.7% | +30.6% (partial) |
| gpt-oss-20b | +28.1% | +15.4% | −1.9% |
| nemotron-3-ultra | +25.3% | −4.2% | −4.5% |

The reasoning models often wrote more output on toonx, and on small tables that extra output outweighs the input saving.

**Through the gateway** (gpt-oss-20b, 120 synthetic questions): input −22.8%, net saving +12.3% [−7.2, +29.0], accuracy inconclusive (−5.8pp [−11.7, +0.0]). The gateway sent 110 tool results as toonx and 10 as JSON, and lost 7 answers against JSON, as the benchmark's own toonx did on the same 120 questions.

**Accuracy against compact JSON** (non-inferiority margin 3pp; the JSON-vs-JSON noise floor is in the tables):

| model | payload Q&A | synthetic | WikiTableQuestions |
|---|---|---|---|
| glm-5.3-flash | non-inferior (−0.7pp) | non-inferior (+0.0pp) | inconclusive (−3.0pp [−9.0, +3.0]) |
| nemotron-3-ultra | non-inferior (+0.0pp) | non-inferior (−0.4pp) | inconclusive (−8.0pp [−14.0, −2.0]) |
| gpt-oss-20b | inconclusive (+2.2pp) | **worse than JSON** (−7.8pp [−12.2, −3.9]) | inconclusive (−3.0pp) |
| llama-3.2-90b-vision | **worse than JSON** (−13.7pp [−22.3, −5.8]) | **worse than JSON** (−12.2pp [−17.4, −7.0]) | inconclusive (partial, 43 questions) |

**What this supports:** toonx reliably cuts input tokens, but whether it saves money and keeps answers right depends on the model and the data. On 2 of 4 models it was worse than JSON on at least one set, and on 2 of 4 the net saving fell below zero on at least one set. A route should be measured before it is switched; on this evidence, the gateway's test switched off the two models where toonx hurt and held the other two for more traffic.

## Disclosures

- **Models:** four open models on NVIDIA NIM's free tier. No Claude or OpenAI-hosted model was tested, and the Mistral and Google slots had no available model.
- **nemotron-3-super was retired by NVIDIA mid-run** (HTTP 410 from 2026-10-03T09:00Z). Its 76 payload questions are reported as partial; nemotron-3-ultra replaced it in every later stage ([Amendment 9](../VERIFY-PLAN.md#amendment-9-a-model-retired-mid-run-before-any-result-on-its-replacement)). The README's earlier `tabular` figure (−8.7pp) was measured on nemotron-3-super; on nemotron-3-ultra, `tabular` was non-inferior (−0.9pp) with a net saving of −9.8%.
- **llama's WikiTableQuestions stage was stopped early,** by choice, at 42 of 100 questions, after interim results had been seen ([Deviation](../VERIFY-PLAN.md#deviation-llamas-wikitablequestions-stage-stopped-early-2026-10-04-before-the-final-report)).
- **Failed calls:** after the registered retry, 2 calls in the whole run still failed (one llama synthetic, one llama WTQ), and are excluded. Empty replies are scored incorrect: one nemotron-3-ultra toonx reply (it asked to call a tool to fetch more pages) and one gpt-oss JSON reply (it used all 16,384 output tokens reasoning).
- **Earlier checks, reported as they came out:** pilot 2's rule fired, and a diagnostic traced it to the output limit ([Amendment 5](../VERIFY-PLAN.md#amendment-5-pilot-2-failed-its-rule-diagnostic-before-any-result)). On open-ended answers, the blind-judge check did not support "no large drop" ([FREEFORM.md](FREEFORM.md)); glm's judged error share rose by +24.2pp.
- **Payload gate:** under toonx 3's gates, 8 of the 36 payloads would have been sent as JSON by the gateway; the benchmark encodes all of them.
- **Operational changes, none affecting which calls were made or how they were scored:** batches of growing size; concurrency 12, then a separate pool per model from stage 2 on; llama's synthetic and WTQ calls in their own process and files, merged before any report. Records from payload batch 1 lack start time, finish reason and reasoning text.
- **The multi-turn tables are computed, not measured.** No multi-turn conversation was run.

## Promotion replay

```sh
go run ./cmd/bench promotion-replay -in bench/results/verify-2026-10/payloads.jsonl,bench/results/verify-2026-10/synthetic.jsonl,bench/results/verify-2026-10/wtq.jsonl
```

Each question answered by json-compact, toonx and json-compact-2 is one three-arm shadow sample. Agreement uses the gateway's comparator on the replies; Tier 1 is "scored correct". Thresholds: first look at 200 samples, then every 100, z=2.5, ε=0.02; minimum net savings 15% with output ×4. Questions arrive in a fixed shuffled order (seed 1).

| model | samples | look at | codec agreement | noise floor | bounds of difference | Tier 1 b / c | input saved | net saved | state | reason |
|---|---:|---:|---:|---:|---|---|---:|---:|---|---|
| nim:meta/llama-3.2-90b-vision-instruct | 410 | 200 | 0.623 | 0.965 | [-0.428, -0.257] | 29 / 6 | 29.2% | 29.1% | OFF | rejected: n=200, agreement 0.623 vs noise floor 0.965 (upper bound -0.257 < −ε) |
| nim:nvidia/nemotron-3-super-120b-a12b | 76 | 76 | 0.921 | 0.987 | [-0.137, +0.005] | 5 / 1 | 26.4% | 13.1% | SHADOW | collecting: 76/200 samples |
| nim:nvidia/nemotron-3-ultra-550b-a55b | 469 | 200 | 0.955 | 0.980 | [-0.057, +0.007] | 6 / 2 | 22.2% | 5.7% | SHADOW | collecting: n=200, bounds [-0.057, +0.007] straddle −ε |
| nim:nvidia/nemotron-3-ultra-550b-a55b | 469 | 300 | 0.965 | 0.983 | [-0.041, +0.005] | 7 / 2 | 22.5% | 4.4% | SHADOW | collecting: n=300, bounds [-0.041, +0.005] straddle −ε |
| nim:nvidia/nemotron-3-ultra-550b-a55b | 469 | 400 | 0.958 | 0.983 | [-0.047, -0.003] | 11 / 3 | 23.0% | 7.1% | SHADOW | collecting: n=400, bounds [-0.047, -0.003] straddle −ε |
| nim:openai/gpt-oss-20b | 469 | 200 | 0.860 | 0.925 | [-0.120, -0.010] | 16 / 10 | 24.2% | 17.7% | SHADOW | collecting: n=200, bounds [-0.120, -0.010] straddle −ε |
| nim:openai/gpt-oss-20b | 469 | 300 | 0.862 | 0.920 | [-0.103, -0.013] | 23 / 13 | 24.7% | 17.0% | SHADOW | collecting: n=300, bounds [-0.103, -0.013] straddle −ε |
| nim:openai/gpt-oss-20b | 469 | 400 | 0.856 | 0.922 | [-0.106, -0.026] | 30 / 18 | 25.2% | 17.8% | OFF | rejected: n=400, agreement 0.856 vs noise floor 0.922 (upper bound -0.026 < −ε) |
| nim:z-ai/glm-5.3-flash | 469 | 200 | 0.960 | 0.985 | [-0.057, +0.007] | 2 / 2 | 24.0% | 21.5% | SHADOW | collecting: n=200, bounds [-0.057, +0.007] straddle −ε |
| nim:z-ai/glm-5.3-flash | 469 | 300 | 0.970 | 0.990 | [-0.043, +0.003] | 3 / 2 | 24.6% | 20.7% | SHADOW | collecting: n=300, bounds [-0.043, +0.003] straddle −ε |
| nim:z-ai/glm-5.3-flash | 469 | 400 | 0.965 | 0.988 | [-0.044, -0.001] | 6 / 3 | 25.1% | 21.8% | SHADOW | collecting: n=400, bounds [-0.044, -0.001] straddle −ε |

## Stage 1: payload Q&A (real public-API responses)

```sh
go run ./cmd/bench verify-report -in bench/results/verify-2026-10/payloads.jsonl
```

Intervals are 95% percentile bootstrap over questions (2000 resamples, seed 1). Accuracy verdicts use a 3pp non-inferiority margin. Token figures are provider-reported usage, paired by question; only questions where both arms succeeded count.

### Accuracy

| model | format | n | acc JSON | acc format | Δacc [95% CI] | answers that flip | verdict |
|---|---|---:|---:|---:|---|---:|---|
| nim:meta/llama-3.2-90b-vision-instruct | json-compact-2 | 139 | 77.0% | 77.0% | +0.0pp [+0.0, +0.0] | 0.0% | noise floor |
| nim:meta/llama-3.2-90b-vision-instruct | toonx | 139 | 77.0% | 63.3% | -13.7pp [-22.3, -5.8] | 25.2% | worse than JSON |
| nim:nvidia/nemotron-3-super-120b-a12b | json-compact-2 | 76 | 98.7% | 98.7% | +0.0pp [+0.0, +0.0] | 0.0% | noise floor |
| nim:nvidia/nemotron-3-super-120b-a12b | toonx | 76 | 98.7% | 93.4% | -5.3pp [-11.8, +0.0] | 7.9% | inconclusive |
| nim:nvidia/nemotron-3-ultra-550b-a55b | json-compact-2 | 139 | 98.6% | 99.3% | +0.7pp [+0.0, +2.2] | 0.7% | noise floor |
| nim:nvidia/nemotron-3-ultra-550b-a55b | toonx | 139 | 98.6% | 98.6% | +0.0pp [-2.9, +2.9] | 2.9% | non-inferior within 3pp |
| nim:openai/gpt-oss-20b | json-compact-2 | 139 | 89.9% | 92.1% | +2.2pp [-2.2, +6.5] | 6.5% | noise floor |
| nim:openai/gpt-oss-20b | toonx | 139 | 89.9% | 92.1% | +2.2pp [-4.3, +8.6] | 15.1% | inconclusive |
| nim:z-ai/glm-5.3-flash | json-compact-2 | 139 | 99.3% | 99.3% | +0.0pp [+0.0, +0.0] | 0.0% | noise floor |
| nim:z-ai/glm-5.3-flash | toonx | 139 | 99.3% | 98.6% | -0.7pp [-2.2, +0.0] | 0.7% | non-inferior within 3pp |

### Tokens

| model | format | n | input tokens | output tokens | reasoning tokens | answer tokens | net saving, output ×4 | net saving, output ×1 | latency ratio (median) |
|---|---|---:|---|---|---:|---:|---|---|---:|
| nim:meta/llama-3.2-90b-vision-instruct | toonx | 139 | -30.4% [-32.9, -28.0] | -8.7% [-18.6, +1.8] | – | – | +30.3% [+27.9, +32.9] | +30.4% [+28.0, +32.9] | ×1.04 |
| nim:nvidia/nemotron-3-super-120b-a12b | toonx | 76 | -26.4% [-30.2, -22.6] | +28.9% [-28.4, +131.4] | +29.5% | -4.8% | +14.1% [+1.3, +27.4] | +22.7% [+17.3, +28.3] | ×1.16 |
| nim:nvidia/nemotron-3-ultra-550b-a55b | toonx | 139 | -25.0% [-27.5, -22.6] | -26.3% [-43.1, -6.2] | -26.8% (est.) | -23.7% | +25.3% [+20.5, +30.5] | +25.1% [+22.4, +27.7] | ×0.74 |
| nim:openai/gpt-oss-20b | toonx | 139 | -27.2% [-29.9, -24.7] | -35.2% [-49.1, -15.9] | -37.6% (est.) | -4.4% | +28.1% [+24.9, +31.1] | +27.4% [+24.9, +30.0] | ×0.93 |
| nim:z-ai/glm-5.3-flash | toonx | 139 | -26.5% [-29.1, -23.9] | -34.1% [-45.0, -20.7] | -35.7% | +9.0% | +27.1% [+24.5, +29.7] | +26.6% [+24.1, +29.2] | ×0.97 |

Input and output columns are the change against `json-compact` (negative input = saving). Net saving is the reduction in input + k·output tokens; k is the output/input price ratio.

### Multi-turn projection (computed, not measured)

| model | format | n | 1 turn | 3 turns | 10 turns | 3 turns, resends at 0.1× | 10 turns, resends at 0.1× |
|---|---|---:|---|---|---|---|---|
| nim:meta/llama-3.2-90b-vision-instruct | toonx | 139 | +30.3% [+27.9, +32.9] | +30.4% [+27.9, +32.9] | +30.4% [+28.0, +32.9] | +30.3% [+27.9, +32.9] | +30.4% [+27.9, +32.9] |
| nim:nvidia/nemotron-3-super-120b-a12b | toonx | 76 | +14.1% [+1.3, +27.4] | +21.5% [+15.5, +28.0] | +24.8% [+20.7, +29.2] | +15.7% [+4.5, +27.6] | +19.1% [+11.0, +27.8] |
| nim:nvidia/nemotron-3-ultra-550b-a55b | toonx | 139 | +25.3% [+20.5, +30.5] | +25.1% [+22.2, +28.0] | +25.1% [+22.6, +27.6] | +25.3% [+21.0, +29.9] | +25.2% [+21.9, +28.8] |
| nim:openai/gpt-oss-20b | toonx | 139 | +28.1% [+24.9, +31.1] | +27.5% [+24.9, +30.1] | +27.3% [+24.8, +29.9] | +27.9% [+25.0, +30.8] | +27.7% [+25.0, +30.4] |
| nim:z-ai/glm-5.3-flash | toonx | 139 | +27.1% [+24.5, +29.7] | +26.7% [+24.2, +29.3] | +26.5% [+24.0, +29.2] | +27.0% [+24.5, +29.6] | +26.8% [+24.3, +29.4] |

Net saving with output ×4 when the same tool result is sent on each of T turns and the output difference is paid once. At 0.1×, every resend after the first is billed as cached input. These are projections from single-turn calls; no multi-turn conversation was run.

### Input savings by data set

| model | format | data set | rows | n | input tokens |
|---|---|---|---:|---:|---|
| nim:meta/llama-3.2-90b-vision-instruct | toonx | payload | 5 | 8 | -24.4% [-30.4, -15.6] |
| nim:meta/llama-3.2-90b-vision-instruct | toonx | payload | 10 | 20 | -31.9% [-38.5, -24.8] |
| nim:meta/llama-3.2-90b-vision-instruct | toonx | payload | 20 | 111 | -30.7% [-33.4, -28.0] |
| nim:nvidia/nemotron-3-super-120b-a12b | toonx | payload | 5 | 8 | -22.0% [-28.1, -13.2] |
| nim:nvidia/nemotron-3-super-120b-a12b | toonx | payload | 10 | 14 | -31.8% [-41.4, -22.3] |
| nim:nvidia/nemotron-3-super-120b-a12b | toonx | payload | 20 | 54 | -25.1% [-29.5, -20.4] |
| nim:nvidia/nemotron-3-ultra-550b-a55b | toonx | payload | 5 | 8 | -22.0% [-28.1, -13.2] |
| nim:nvidia/nemotron-3-ultra-550b-a55b | toonx | payload | 10 | 20 | -29.1% [-36.0, -21.3] |
| nim:nvidia/nemotron-3-ultra-550b-a55b | toonx | payload | 20 | 111 | -24.2% [-26.7, -21.6] |
| nim:openai/gpt-oss-20b | toonx | payload | 5 | 8 | -23.2% [-29.7, -13.9] |
| nim:openai/gpt-oss-20b | toonx | payload | 10 | 20 | -30.1% [-36.7, -22.6] |
| nim:openai/gpt-oss-20b | toonx | payload | 20 | 111 | -26.8% [-29.6, -24.0] |
| nim:z-ai/glm-5.3-flash | toonx | payload | 5 | 8 | -23.1% [-29.4, -13.8] |
| nim:z-ai/glm-5.3-flash | toonx | payload | 10 | 20 | -29.6% [-36.6, -21.8] |
| nim:z-ai/glm-5.3-flash | toonx | payload | 20 | 111 | -25.9% [-28.6, -23.2] |

### Calls

| model | format | calls | attempts | failed after retries (excluded) | model retired (excluded) | empty replies (scored incorrect) | served encoded by the gateway |
|---|---|---:|---:|---:|---:|---:|---:|
| nim:meta/llama-3.2-90b-vision-instruct | json-compact | 139 | 147 | 0 | 0 | 0 | – |
| nim:meta/llama-3.2-90b-vision-instruct | json-compact-2 | 139 | 147 | 0 | 0 | 0 | – |
| nim:meta/llama-3.2-90b-vision-instruct | toonx | 139 | 149 | 0 | 0 | 0 | – |
| nim:nvidia/nemotron-3-super-120b-a12b | json-compact | 76 | 76 | 0 | 29 | 0 | – |
| nim:nvidia/nemotron-3-super-120b-a12b | json-compact-2 | 76 | 76 | 0 | 29 | 0 | – |
| nim:nvidia/nemotron-3-super-120b-a12b | toonx | 76 | 76 | 0 | 29 | 0 | – |
| nim:nvidia/nemotron-3-ultra-550b-a55b | json-compact | 139 | 139 | 0 | 0 | 0 | – |
| nim:nvidia/nemotron-3-ultra-550b-a55b | json-compact-2 | 139 | 139 | 0 | 0 | 0 | – |
| nim:nvidia/nemotron-3-ultra-550b-a55b | toonx | 139 | 139 | 0 | 0 | 1 | – |
| nim:openai/gpt-oss-20b | json-compact | 139 | 139 | 0 | 0 | 0 | – |
| nim:openai/gpt-oss-20b | json-compact-2 | 139 | 140 | 0 | 0 | 0 | – |
| nim:openai/gpt-oss-20b | toonx | 139 | 140 | 0 | 0 | 0 | – |
| nim:z-ai/glm-5.3-flash | toonx | 139 | 139 | 0 | 0 | 0 | – |
| nim:z-ai/glm-5.3-flash | json-compact | 139 | 141 | 0 | 0 | 0 | – |
| nim:z-ai/glm-5.3-flash | json-compact-2 | 139 | 140 | 0 | 0 | 0 | – |

## Stage 2 and 5: synthetic tables (with `tabular` on nemotron-3-ultra)

```sh
go run ./cmd/bench verify-report -in bench/results/verify-2026-10/synthetic.jsonl
```

Intervals are 95% percentile bootstrap over questions (2000 resamples, seed 1). Accuracy verdicts use a 3pp non-inferiority margin. Token figures are provider-reported usage, paired by question; only questions where both arms succeeded count.

### Accuracy

| model | format | n | acc JSON | acc format | Δacc [95% CI] | answers that flip | verdict |
|---|---|---:|---:|---:|---|---:|---|
| nim:meta/llama-3.2-90b-vision-instruct | json-compact-2 | 229 | 68.6% | 67.7% | -0.9pp [-2.2, +0.0] | 0.9% | noise floor |
| nim:meta/llama-3.2-90b-vision-instruct | toonx | 230 | 68.7% | 56.5% | -12.2pp [-17.4, -7.0] | 20.0% | worse than JSON |
| nim:nvidia/nemotron-3-ultra-550b-a55b | json-compact-2 | 230 | 99.6% | 99.6% | +0.0pp [+0.0, +0.0] | 0.0% | noise floor |
| nim:nvidia/nemotron-3-ultra-550b-a55b | toonx | 230 | 99.6% | 99.1% | -0.4pp [-2.2, +0.9] | 1.3% | non-inferior within 3pp |
| nim:nvidia/nemotron-3-ultra-550b-a55b | tabular | 230 | 99.6% | 98.7% | -0.9pp [-2.6, +0.9] | 1.7% | non-inferior within 3pp |
| nim:openai/gpt-oss-20b | json-compact-2 | 230 | 96.5% | 94.3% | -2.2pp [-5.2, +0.4] | 4.8% | noise floor |
| nim:openai/gpt-oss-20b | toonx | 230 | 96.5% | 88.7% | -7.8pp [-12.2, -3.9] | 10.4% | worse than JSON |
| nim:z-ai/glm-5.3-flash | json-compact-2 | 230 | 100.0% | 99.6% | -0.4pp [-1.7, +0.0] | 0.4% | noise floor |
| nim:z-ai/glm-5.3-flash | toonx | 230 | 100.0% | 100.0% | +0.0pp [+0.0, +0.0] | 0.0% | non-inferior within 3pp |

### Tokens

| model | format | n | input tokens | output tokens | reasoning tokens | answer tokens | net saving, output ×4 | net saving, output ×1 | latency ratio (median) |
|---|---|---:|---|---|---:|---:|---|---|---:|
| nim:meta/llama-3.2-90b-vision-instruct | toonx | 230 | -29.8% [-30.8, -28.8] | +3.3% [+0.1, +8.1] | – | – | +29.7% [+28.7, +30.7] | +29.8% [+28.8, +30.8] | ×0.98 |
| nim:nvidia/nemotron-3-ultra-550b-a55b | toonx | 230 | -20.4% [-21.0, -19.9] | +69.4% [+41.1, +102.9] | +62.6% (est.) | +92.8% | -4.2% [-12.9, +3.3] | +12.7% [+9.8, +15.1] | ×1.28 |
| nim:nvidia/nemotron-3-ultra-550b-a55b | tabular | 230 | -23.4% [-24.4, -22.4] | +97.5% [+60.0, +137.5] | +93.9% (est.) | +110.0% | -9.8% [-20.1, +0.5] | +13.0% [+9.4, +16.4] | ×1.18 |
| nim:openai/gpt-oss-20b | toonx | 230 | -23.3% [-24.2, -22.4] | -2.8% [-25.8, +30.4] | -2.9% (est.) | +0.3% | +15.4% [+4.6, +24.3] | +20.5% [+16.9, +23.8] | ×1.15 |
| nim:z-ai/glm-5.3-flash | toonx | 230 | -23.4% [-24.2, -22.5] | +12.3% [-4.7, +31.5] | +12.5% | +0.6% | +17.3% [+14.3, +20.2] | +21.6% [+20.4, +22.8] | ×1.02 |

Input and output columns are the change against `json-compact` (negative input = saving). Net saving is the reduction in input + k·output tokens; k is the output/input price ratio.

### Multi-turn projection (computed, not measured)

| model | format | n | 1 turn | 3 turns | 10 turns | 3 turns, resends at 0.1× | 10 turns, resends at 0.1× |
|---|---|---:|---|---|---|---|---|
| nim:meta/llama-3.2-90b-vision-instruct | toonx | 230 | +29.7% [+28.7, +30.7] | +29.8% [+28.7, +30.8] | +29.8% [+28.8, +30.8] | +29.7% [+28.7, +30.7] | +29.8% [+28.7, +30.8] |
| nim:nvidia/nemotron-3-ultra-550b-a55b | toonx | 230 | -4.2% [-12.9, +3.3] | +10.4% [+6.7, +13.5] | +17.2% [+15.8, +18.3] | -1.1% [-8.8, +5.5] | +5.5% [+0.1, +10.1] |
| nim:openai/gpt-oss-20b | toonx | 230 | +15.4% [+4.6, +24.3] | +19.8% [+15.0, +23.8] | +22.1% [+20.3, +23.7] | +16.3% [+6.7, +24.2] | +18.2% [+11.5, +24.0] |
| nim:z-ai/glm-5.3-flash | toonx | 230 | +17.3% [+14.3, +20.2] | +21.1% [+19.7, +22.5] | +22.7% [+21.7, +23.6] | +18.2% [+15.5, +20.7] | +19.9% [+18.0, +21.7] |

Net saving with output ×4 when the same tool result is sent on each of T turns and the output difference is paid once. At 0.1×, every resend after the first is billed as cached input. These are projections from single-turn calls; no multi-turn conversation was run.

### Input savings by data set

| model | format | data set | rows | n | input tokens |
|---|---|---|---:|---:|---|
| nim:meta/llama-3.2-90b-vision-instruct | toonx | employees | 30 | 30 | -31.4% [-31.5, -31.3] |
| nim:meta/llama-3.2-90b-vision-instruct | toonx | employees | 120 | 30 | -38.2% [-38.2, -38.1] |
| nim:meta/llama-3.2-90b-vision-instruct | toonx | logs | 30 | 30 | -24.1% [-24.2, -24.0] |
| nim:meta/llama-3.2-90b-vision-instruct | toonx | logs | 120 | 30 | -29.9% [-30.0, -29.9] |
| nim:meta/llama-3.2-90b-vision-instruct | toonx | orders | 30 | 35 | -25.8% [-25.8, -25.7] |
| nim:meta/llama-3.2-90b-vision-instruct | toonx | orders | 120 | 35 | -32.5% [-32.6, -32.5] |
| nim:meta/llama-3.2-90b-vision-instruct | toonx | search | 30 | 20 | -16.8% [-16.8, -16.8] |
| nim:meta/llama-3.2-90b-vision-instruct | toonx | search | 120 | 20 | -20.7% [-20.8, -20.6] |
| nim:nvidia/nemotron-3-ultra-550b-a55b | toonx | employees | 30 | 30 | -21.1% [-21.1, -21.0] |
| nim:nvidia/nemotron-3-ultra-550b-a55b | toonx | employees | 120 | 30 | -25.6% [-25.7, -25.6] |
| nim:nvidia/nemotron-3-ultra-550b-a55b | toonx | logs | 30 | 30 | -16.3% [-16.3, -16.2] |
| nim:nvidia/nemotron-3-ultra-550b-a55b | toonx | logs | 120 | 30 | -20.4% [-20.4, -20.4] |
| nim:nvidia/nemotron-3-ultra-550b-a55b | toonx | orders | 30 | 35 | -16.0% [-16.1, -15.8] |
| nim:nvidia/nemotron-3-ultra-550b-a55b | toonx | orders | 120 | 35 | -20.9% [-20.9, -20.9] |
| nim:nvidia/nemotron-3-ultra-550b-a55b | toonx | search | 30 | 20 | -13.1% [-13.1, -13.0] |
| nim:nvidia/nemotron-3-ultra-550b-a55b | toonx | search | 120 | 20 | -16.6% [-16.6, -16.5] |
| nim:nvidia/nemotron-3-ultra-550b-a55b | tabular | employees | 30 | 30 | -29.7% [-29.8, -29.7] |
| nim:nvidia/nemotron-3-ultra-550b-a55b | tabular | employees | 120 | 30 | -33.0% [-33.0, -32.9] |
| nim:nvidia/nemotron-3-ultra-550b-a55b | tabular | logs | 30 | 30 | -18.5% [-18.6, -18.5] |
| nim:nvidia/nemotron-3-ultra-550b-a55b | tabular | logs | 120 | 30 | -21.5% [-21.6, -21.5] |
| nim:nvidia/nemotron-3-ultra-550b-a55b | tabular | orders | 30 | 35 | -19.0% [-19.0, -19.0] |
| nim:nvidia/nemotron-3-ultra-550b-a55b | tabular | orders | 120 | 35 | -22.7% [-22.7, -22.6] |
| nim:nvidia/nemotron-3-ultra-550b-a55b | tabular | search | 30 | 20 | -13.5% [-13.5, -13.5] |
| nim:nvidia/nemotron-3-ultra-550b-a55b | tabular | search | 120 | 20 | -15.7% [-15.8, -15.7] |
| nim:openai/gpt-oss-20b | toonx | employees | 30 | 30 | -23.2% [-23.4, -23.1] |
| nim:openai/gpt-oss-20b | toonx | employees | 120 | 30 | -30.7% [-30.7, -30.6] |
| nim:openai/gpt-oss-20b | toonx | logs | 30 | 30 | -19.1% [-19.1, -19.0] |
| nim:openai/gpt-oss-20b | toonx | logs | 120 | 30 | -25.3% [-25.3, -25.2] |
| nim:openai/gpt-oss-20b | toonx | orders | 30 | 35 | -18.0% [-18.1, -17.8] |
| nim:openai/gpt-oss-20b | toonx | orders | 120 | 35 | -25.1% [-25.1, -25.0] |
| nim:openai/gpt-oss-20b | toonx | search | 30 | 20 | -10.8% [-10.9, -10.8] |
| nim:openai/gpt-oss-20b | toonx | search | 120 | 20 | -14.9% [-15.0, -14.9] |
| nim:z-ai/glm-5.3-flash | toonx | employees | 30 | 30 | -24.3% [-24.4, -24.2] |
| nim:z-ai/glm-5.3-flash | toonx | employees | 120 | 30 | -30.2% [-30.2, -30.1] |
| nim:z-ai/glm-5.3-flash | toonx | logs | 30 | 30 | -19.3% [-19.4, -19.3] |
| nim:z-ai/glm-5.3-flash | toonx | logs | 120 | 30 | -24.5% [-24.5, -24.4] |
| nim:z-ai/glm-5.3-flash | toonx | orders | 30 | 35 | -19.5% [-19.6, -19.4] |
| nim:z-ai/glm-5.3-flash | toonx | orders | 120 | 35 | -25.7% [-25.7, -25.6] |
| nim:z-ai/glm-5.3-flash | toonx | search | 30 | 20 | -11.2% [-11.2, -11.1] |
| nim:z-ai/glm-5.3-flash | toonx | search | 120 | 20 | -14.9% [-14.9, -14.8] |

### Calls

| model | format | calls | attempts | failed after retries (excluded) | model retired (excluded) | empty replies (scored incorrect) | served encoded by the gateway |
|---|---|---:|---:|---:|---:|---:|---:|
| nim:meta/llama-3.2-90b-vision-instruct | json-compact-2 | 230 | 246 | 1 | 0 | 0 | – |
| nim:meta/llama-3.2-90b-vision-instruct | json-compact | 230 | 251 | 0 | 0 | 0 | – |
| nim:meta/llama-3.2-90b-vision-instruct | toonx | 230 | 249 | 0 | 0 | 0 | – |
| nim:nvidia/nemotron-3-ultra-550b-a55b | json-compact | 230 | 230 | 0 | 0 | 0 | – |
| nim:nvidia/nemotron-3-ultra-550b-a55b | json-compact-2 | 230 | 231 | 0 | 0 | 0 | – |
| nim:nvidia/nemotron-3-ultra-550b-a55b | toonx | 230 | 230 | 0 | 0 | 0 | – |
| nim:nvidia/nemotron-3-ultra-550b-a55b | tabular | 230 | 230 | 0 | 0 | 0 | – |
| nim:openai/gpt-oss-20b | json-compact | 230 | 231 | 0 | 0 | 0 | – |
| nim:openai/gpt-oss-20b | json-compact-2 | 230 | 233 | 0 | 0 | 0 | – |
| nim:openai/gpt-oss-20b | toonx | 230 | 232 | 0 | 0 | 0 | – |
| nim:z-ai/glm-5.3-flash | json-compact | 230 | 230 | 0 | 0 | 0 | – |
| nim:z-ai/glm-5.3-flash | json-compact-2 | 230 | 232 | 0 | 0 | 0 | – |
| nim:z-ai/glm-5.3-flash | toonx | 230 | 230 | 0 | 0 | 0 | – |

## Stage 3: the gateway path (gpt-oss-20b)

```sh
go run ./cmd/bench verify-report -in bench/results/verify-2026-10/gateway.jsonl
```

Intervals are 95% percentile bootstrap over questions (2000 resamples, seed 1). Accuracy verdicts use a 3pp non-inferiority margin. Token figures are provider-reported usage, paired by question; only questions where both arms succeeded count.

### Accuracy

| model | format | n | acc JSON | acc format | Δacc [95% CI] | answers that flip | verdict |
|---|---|---:|---:|---:|---|---:|---|
| nim:openai/gpt-oss-20b | gateway | 120 | 95.8% | 90.0% | -5.8pp [-11.7, +0.0] | 10.8% | inconclusive |

### Tokens

| model | format | n | input tokens | output tokens | reasoning tokens | answer tokens | net saving, output ×4 | net saving, output ×1 | latency ratio (median) |
|---|---|---:|---|---|---:|---:|---|---|---:|
| nim:openai/gpt-oss-20b | gateway | 120 | -22.8% [-24.1, -21.5] | +1.7% [-35.2, +64.2] | +1.6% (est.) | +2.4% | +12.3% [-7.2, +29.0] | +18.9% [+12.2, +25.2] | ×1.09 |

Input and output columns are the change against `json-compact` (negative input = saving). Net saving is the reduction in input + k·output tokens; k is the output/input price ratio.

### Multi-turn projection (computed, not measured)

| model | format | n | 1 turn | 3 turns | 10 turns | 3 turns, resends at 0.1× | 10 turns, resends at 0.1× |
|---|---|---:|---|---|---|---|---|
| nim:openai/gpt-oss-20b | gateway | 120 | +12.3% [-7.2, +29.0] | +17.9% [+9.3, +25.8] | +21.1% [+17.9, +24.1] | +13.4% [-3.9, +28.4] | +15.8% [+3.5, +27.0] |

Net saving with output ×4 when the same tool result is sent on each of T turns and the output difference is paid once. At 0.1×, every resend after the first is billed as cached input. These are projections from single-turn calls; no multi-turn conversation was run.

### Input savings by data set

| model | format | data set | rows | n | input tokens |
|---|---|---|---:|---:|---|
| nim:openai/gpt-oss-20b | gateway | employees | 30 | 12 | -23.5% [-23.5, -23.4] |
| nim:openai/gpt-oss-20b | gateway | employees | 120 | 12 | -30.6% [-30.7, -30.6] |
| nim:openai/gpt-oss-20b | gateway | logs | 30 | 18 | -19.0% [-19.1, -19.0] |
| nim:openai/gpt-oss-20b | gateway | logs | 120 | 18 | -25.2% [-25.3, -25.2] |
| nim:openai/gpt-oss-20b | gateway | orders | 30 | 21 | -18.1% [-18.2, -17.9] |
| nim:openai/gpt-oss-20b | gateway | orders | 120 | 21 | -25.1% [-25.1, -25.1] |
| nim:openai/gpt-oss-20b | gateway | search | 30 | 10 | -0.0% [-0.0, -0.0] |
| nim:openai/gpt-oss-20b | gateway | search | 120 | 8 | -14.9% [-14.9, -14.9] |

### Calls

| model | format | calls | attempts | failed after retries (excluded) | model retired (excluded) | empty replies (scored incorrect) | served encoded by the gateway |
|---|---|---:|---:|---:|---:|---:|---:|
| nim:openai/gpt-oss-20b | json-compact | 120 | 120 | 0 | 0 | 1 | – |
| nim:openai/gpt-oss-20b | gateway | 120 | 120 | 0 | 0 | 0 | 110 |

## Stage 4: WikiTableQuestions

```sh
go run ./cmd/bench verify-report -in bench/results/verify-2026-10/wtq.jsonl
```

Intervals are 95% percentile bootstrap over questions (2000 resamples, seed 1). Accuracy verdicts use a 3pp non-inferiority margin. Token figures are provider-reported usage, paired by question; only questions where both arms succeeded count.

### Accuracy

| model | format | n | acc JSON | acc format | Δacc [95% CI] | answers that flip | verdict |
|---|---|---:|---:|---:|---|---:|---|
| nim:meta/llama-3.2-90b-vision-instruct | json-compact-2 | 42 | 50.0% | 52.4% | +2.4pp [+0.0, +7.1] | 2.4% | noise floor |
| nim:meta/llama-3.2-90b-vision-instruct | toonx | 43 | 48.8% | 48.8% | +0.0pp [-11.6, +11.6] | 14.0% | inconclusive |
| nim:nvidia/nemotron-3-ultra-550b-a55b | json-compact-2 | 100 | 86.0% | 83.0% | -3.0pp [-7.0, +0.0] | 3.0% | noise floor |
| nim:nvidia/nemotron-3-ultra-550b-a55b | toonx | 100 | 86.0% | 78.0% | -8.0pp [-14.0, -2.0] | 10.0% | inconclusive |
| nim:openai/gpt-oss-20b | json-compact-2 | 100 | 80.0% | 78.0% | -2.0pp [-8.0, +4.0] | 10.0% | noise floor |
| nim:openai/gpt-oss-20b | toonx | 100 | 80.0% | 77.0% | -3.0pp [-10.0, +5.0] | 15.0% | inconclusive |
| nim:z-ai/glm-5.3-flash | json-compact-2 | 100 | 87.0% | 88.0% | +1.0pp [-3.0, +6.0] | 5.0% | noise floor |
| nim:z-ai/glm-5.3-flash | toonx | 100 | 87.0% | 84.0% | -3.0pp [-9.0, +3.0] | 9.0% | inconclusive |

### Tokens

| model | format | n | input tokens | output tokens | reasoning tokens | answer tokens | net saving, output ×4 | net saving, output ×1 | latency ratio (median) |
|---|---|---:|---|---|---:|---:|---|---|---:|
| nim:meta/llama-3.2-90b-vision-instruct | toonx | 43 | -30.9% [-35.9, -25.6] | +15.5% [+1.7, +31.7] | – | – | +30.6% [+25.2, +35.6] | +30.8% [+25.5, +35.8] | ×0.75 |
| nim:nvidia/nemotron-3-ultra-550b-a55b | toonx | 100 | -30.2% [-34.3, -26.0] | +48.3% [+18.0, +85.4] | +47.8% (est.) | +51.9% | -4.5% [-17.9, +8.3] | +17.2% [+11.7, +22.2] | ×1.19 |
| nim:openai/gpt-oss-20b | toonx | 100 | -28.4% [-32.4, -24.1] | +37.7% [-3.5, +109.0] | +39.2% (est.) | -3.6% | -1.9% [-27.1, +15.5] | +16.8% [+6.5, +24.4] | ×1.27 |
| nim:z-ai/glm-5.3-flash | toonx | 100 | -30.4% [-34.7, -26.0] | -11.4% [-26.4, +7.1] | -11.5% | -8.6% | +25.6% [+20.1, +30.2] | +28.9% [+24.7, +33.1] | ×1.02 |

Input and output columns are the change against `json-compact` (negative input = saving). Net saving is the reduction in input + k·output tokens; k is the output/input price ratio.

### Multi-turn projection (computed, not measured)

| model | format | n | 1 turn | 3 turns | 10 turns | 3 turns, resends at 0.1× | 10 turns, resends at 0.1× |
|---|---|---:|---|---|---|---|---|
| nim:meta/llama-3.2-90b-vision-instruct | toonx | 43 | +30.6% [+25.2, +35.6] | +30.8% [+25.5, +35.8] | +30.9% [+25.5, +35.9] | +30.6% [+25.3, +35.6] | +30.7% [+25.4, +35.7] |
| nim:nvidia/nemotron-3-ultra-550b-a55b | toonx | 100 | -4.5% [-17.9, +8.3] | +13.8% [+7.2, +19.9] | +24.4% [+20.1, +28.1] | -1.0% [-13.0, +10.4] | +7.1% [-1.6, +15.6] |
| nim:openai/gpt-oss-20b | toonx | 100 | -1.9% [-27.1, +15.5] | +13.8% [+1.4, +22.9] | +23.2% [+17.0, +28.3] | +1.0% [-21.3, +16.8] | +8.0% [-8.5, +19.9] |
| nim:z-ai/glm-5.3-flash | toonx | 100 | +25.6% [+20.1, +30.2] | +28.5% [+24.2, +32.6] | +29.8% [+25.6, +34.0] | +26.2% [+21.1, +30.5] | +27.5% [+23.0, +31.6] |

Net saving with output ×4 when the same tool result is sent on each of T turns and the output difference is paid once. At 0.1×, every resend after the first is billed as cached input. These are projections from single-turn calls; no multi-turn conversation was run.

### Input savings by data set

| model | format | data set | rows | n | input tokens |
|---|---|---|---:|---:|---|
| nim:meta/llama-3.2-90b-vision-instruct | toonx | wtq | 15 | 1 | -41.5% [-41.5, -41.5] |
| nim:meta/llama-3.2-90b-vision-instruct | toonx | wtq | 16 | 6 | -28.6% [-40.5, -21.2] |
| nim:meta/llama-3.2-90b-vision-instruct | toonx | wtq | 17 | 4 | -24.2% [-30.2, -17.5] |
| nim:meta/llama-3.2-90b-vision-instruct | toonx | wtq | 18 | 3 | -28.2% [-30.8, -22.0] |
| nim:meta/llama-3.2-90b-vision-instruct | toonx | wtq | 19 | 4 | -28.3% [-34.5, -24.0] |
| nim:meta/llama-3.2-90b-vision-instruct | toonx | wtq | 20 | 2 | -28.5% [-29.1, -27.5] |
| nim:meta/llama-3.2-90b-vision-instruct | toonx | wtq | 21 | 2 | -31.0% [-37.7, -18.4] |
| nim:meta/llama-3.2-90b-vision-instruct | toonx | wtq | 22 | 2 | -29.8% [-34.4, -24.6] |
| nim:meta/llama-3.2-90b-vision-instruct | toonx | wtq | 23 | 1 | -37.5% [-37.5, -37.5] |
| nim:meta/llama-3.2-90b-vision-instruct | toonx | wtq | 24 | 2 | -26.1% [-26.6, -25.6] |
| nim:meta/llama-3.2-90b-vision-instruct | toonx | wtq | 26 | 1 | -42.0% [-42.0, -42.0] |
| nim:meta/llama-3.2-90b-vision-instruct | toonx | wtq | 27 | 2 | -30.7% [-30.7, -30.7] |
| nim:meta/llama-3.2-90b-vision-instruct | toonx | wtq | 28 | 1 | -15.5% [-15.5, -15.5] |
| nim:meta/llama-3.2-90b-vision-instruct | toonx | wtq | 30 | 5 | -29.0% [-38.3, -22.3] |
| nim:meta/llama-3.2-90b-vision-instruct | toonx | wtq | 35 | 1 | -41.7% [-41.7, -41.7] |
| nim:meta/llama-3.2-90b-vision-instruct | toonx | wtq | 46 | 2 | -34.5% [-34.6, -34.4] |
| nim:meta/llama-3.2-90b-vision-instruct | toonx | wtq | 50 | 1 | -26.9% [-26.9, -26.9] |
| nim:meta/llama-3.2-90b-vision-instruct | toonx | wtq | 60 | 1 | -16.6% [-16.6, -16.6] |
| nim:meta/llama-3.2-90b-vision-instruct | toonx | wtq | 111 | 1 | -47.8% [-47.8, -47.8] |
| nim:meta/llama-3.2-90b-vision-instruct | toonx | wtq | 118 | 1 | -42.1% [-42.1, -42.1] |
| nim:nvidia/nemotron-3-ultra-550b-a55b | toonx | wtq | 15 | 3 | -35.4% [-40.8, -13.5] |
| nim:nvidia/nemotron-3-ultra-550b-a55b | toonx | wtq | 16 | 10 | -20.5% [-29.3, -14.4] |
| nim:nvidia/nemotron-3-ultra-550b-a55b | toonx | wtq | 17 | 9 | -15.4% [-21.4, -9.7] |
| nim:nvidia/nemotron-3-ultra-550b-a55b | toonx | wtq | 18 | 6 | -20.8% [-24.7, -16.6] |
| nim:nvidia/nemotron-3-ultra-550b-a55b | toonx | wtq | 19 | 9 | -24.2% [-29.9, -19.1] |
| nim:nvidia/nemotron-3-ultra-550b-a55b | toonx | wtq | 20 | 4 | -23.9% [-25.8, -21.2] |
| nim:nvidia/nemotron-3-ultra-550b-a55b | toonx | wtq | 21 | 3 | -20.5% [-32.1, -11.2] |
| nim:nvidia/nemotron-3-ultra-550b-a55b | toonx | wtq | 22 | 3 | -23.4% [-27.4, -19.8] |
| nim:nvidia/nemotron-3-ultra-550b-a55b | toonx | wtq | 23 | 1 | -31.6% [-31.6, -31.6] |
| nim:nvidia/nemotron-3-ultra-550b-a55b | toonx | wtq | 24 | 4 | -23.2% [-27.6, -19.7] |
| nim:nvidia/nemotron-3-ultra-550b-a55b | toonx | wtq | 26 | 2 | -31.2% [-33.2, -27.7] |
| nim:nvidia/nemotron-3-ultra-550b-a55b | toonx | wtq | 27 | 5 | -25.0% [-32.3, -20.0] |
| nim:nvidia/nemotron-3-ultra-550b-a55b | toonx | wtq | 28 | 3 | -18.3% [-32.2, -13.0] |
| nim:nvidia/nemotron-3-ultra-550b-a55b | toonx | wtq | 29 | 2 | -21.6% [-21.7, -21.6] |
| nim:nvidia/nemotron-3-ultra-550b-a55b | toonx | wtq | 30 | 7 | -28.8% [-36.6, -21.4] |
| nim:nvidia/nemotron-3-ultra-550b-a55b | toonx | wtq | 31 | 1 | -16.0% [-16.0, -16.0] |
| nim:nvidia/nemotron-3-ultra-550b-a55b | toonx | wtq | 34 | 3 | -29.8% [-31.8, -25.7] |
| nim:nvidia/nemotron-3-ultra-550b-a55b | toonx | wtq | 35 | 4 | -39.3% [-41.4, -35.3] |
| nim:nvidia/nemotron-3-ultra-550b-a55b | toonx | wtq | 36 | 2 | -29.1% [-30.8, -26.0] |
| nim:nvidia/nemotron-3-ultra-550b-a55b | toonx | wtq | 38 | 1 | -37.7% [-37.7, -37.7] |
| nim:nvidia/nemotron-3-ultra-550b-a55b | toonx | wtq | 40 | 1 | -20.6% [-20.6, -20.6] |
| nim:nvidia/nemotron-3-ultra-550b-a55b | toonx | wtq | 42 | 2 | -25.9% [-29.9, -20.6] |
| nim:nvidia/nemotron-3-ultra-550b-a55b | toonx | wtq | 46 | 2 | -29.8% [-29.8, -29.7] |
| nim:nvidia/nemotron-3-ultra-550b-a55b | toonx | wtq | 50 | 2 | -24.8% [-24.9, -24.8] |
| nim:nvidia/nemotron-3-ultra-550b-a55b | toonx | wtq | 56 | 1 | -25.7% [-25.7, -25.7] |
| nim:nvidia/nemotron-3-ultra-550b-a55b | toonx | wtq | 60 | 2 | -17.5% [-23.6, -15.0] |
| nim:nvidia/nemotron-3-ultra-550b-a55b | toonx | wtq | 75 | 1 | -41.3% [-41.3, -41.3] |
| nim:nvidia/nemotron-3-ultra-550b-a55b | toonx | wtq | 95 | 1 | -48.6% [-48.6, -48.6] |
| nim:nvidia/nemotron-3-ultra-550b-a55b | toonx | wtq | 99 | 1 | -38.0% [-38.0, -38.0] |
| nim:nvidia/nemotron-3-ultra-550b-a55b | toonx | wtq | 111 | 2 | -40.3% [-40.3, -40.3] |
| nim:nvidia/nemotron-3-ultra-550b-a55b | toonx | wtq | 118 | 1 | -29.0% [-29.0, -29.0] |
| nim:nvidia/nemotron-3-ultra-550b-a55b | toonx | wtq | 176 | 1 | -44.4% [-44.4, -44.4] |
| nim:nvidia/nemotron-3-ultra-550b-a55b | toonx | wtq | 297 | 1 | -51.9% [-51.9, -51.9] |
| nim:openai/gpt-oss-20b | toonx | wtq | 15 | 3 | -28.2% [-34.0, -5.9] |
| nim:openai/gpt-oss-20b | toonx | wtq | 16 | 10 | -18.0% [-25.4, -12.9] |
| nim:openai/gpt-oss-20b | toonx | wtq | 17 | 9 | -13.2% [-19.3, -7.6] |
| nim:openai/gpt-oss-20b | toonx | wtq | 18 | 6 | -19.0% [-23.3, -14.2] |
| nim:openai/gpt-oss-20b | toonx | wtq | 19 | 9 | -18.8% [-23.3, -15.0] |
| nim:openai/gpt-oss-20b | toonx | wtq | 20 | 4 | -18.8% [-20.0, -17.1] |
| nim:openai/gpt-oss-20b | toonx | wtq | 21 | 3 | -19.6% [-28.9, -9.8] |
| nim:openai/gpt-oss-20b | toonx | wtq | 22 | 3 | -21.6% [-25.8, -16.9] |
| nim:openai/gpt-oss-20b | toonx | wtq | 23 | 1 | -29.2% [-29.2, -29.2] |
| nim:openai/gpt-oss-20b | toonx | wtq | 24 | 4 | -19.8% [-24.5, -17.9] |
| nim:openai/gpt-oss-20b | toonx | wtq | 26 | 2 | -32.0% [-34.2, -28.1] |
| nim:openai/gpt-oss-20b | toonx | wtq | 27 | 5 | -24.5% [-29.8, -20.4] |
| nim:openai/gpt-oss-20b | toonx | wtq | 28 | 3 | -17.7% [-33.3, -12.0] |
| nim:openai/gpt-oss-20b | toonx | wtq | 29 | 2 | -19.4% [-19.5, -19.4] |
| nim:openai/gpt-oss-20b | toonx | wtq | 30 | 7 | -25.1% [-33.0, -17.4] |
| nim:openai/gpt-oss-20b | toonx | wtq | 31 | 1 | -14.7% [-14.7, -14.7] |
| nim:openai/gpt-oss-20b | toonx | wtq | 34 | 3 | -29.3% [-31.2, -25.7] |
| nim:openai/gpt-oss-20b | toonx | wtq | 35 | 4 | -36.1% [-40.5, -34.1] |
| nim:openai/gpt-oss-20b | toonx | wtq | 36 | 2 | -30.9% [-34.4, -24.8] |
| nim:openai/gpt-oss-20b | toonx | wtq | 38 | 1 | -36.3% [-36.3, -36.3] |
| nim:openai/gpt-oss-20b | toonx | wtq | 40 | 1 | -19.6% [-19.6, -19.6] |
| nim:openai/gpt-oss-20b | toonx | wtq | 42 | 2 | -24.0% [-26.6, -20.8] |
| nim:openai/gpt-oss-20b | toonx | wtq | 46 | 2 | -27.1% [-27.2, -27.0] |
| nim:openai/gpt-oss-20b | toonx | wtq | 50 | 2 | -21.6% [-21.6, -21.6] |
| nim:openai/gpt-oss-20b | toonx | wtq | 56 | 1 | -25.9% [-25.9, -25.9] |
| nim:openai/gpt-oss-20b | toonx | wtq | 60 | 2 | -17.1% [-24.2, -14.6] |
| nim:openai/gpt-oss-20b | toonx | wtq | 75 | 1 | -38.9% [-38.9, -38.9] |
| nim:openai/gpt-oss-20b | toonx | wtq | 95 | 1 | -49.9% [-49.9, -49.9] |
| nim:openai/gpt-oss-20b | toonx | wtq | 99 | 1 | -33.9% [-33.9, -33.9] |
| nim:openai/gpt-oss-20b | toonx | wtq | 111 | 2 | -40.9% [-40.9, -40.9] |
| nim:openai/gpt-oss-20b | toonx | wtq | 118 | 1 | -29.8% [-29.8, -29.8] |
| nim:openai/gpt-oss-20b | toonx | wtq | 176 | 1 | -43.1% [-43.1, -43.1] |
| nim:openai/gpt-oss-20b | toonx | wtq | 297 | 1 | -52.0% [-52.0, -52.0] |
| nim:z-ai/glm-5.3-flash | toonx | wtq | 15 | 3 | -33.5% [-39.1, -9.1] |
| nim:z-ai/glm-5.3-flash | toonx | wtq | 16 | 10 | -19.6% [-28.5, -13.1] |
| nim:z-ai/glm-5.3-flash | toonx | wtq | 17 | 9 | -15.3% [-22.3, -8.9] |
| nim:z-ai/glm-5.3-flash | toonx | wtq | 18 | 6 | -21.5% [-26.2, -16.3] |
| nim:z-ai/glm-5.3-flash | toonx | wtq | 19 | 9 | -21.8% [-25.5, -18.6] |
| nim:z-ai/glm-5.3-flash | toonx | wtq | 20 | 4 | -21.6% [-22.4, -20.3] |
| nim:z-ai/glm-5.3-flash | toonx | wtq | 21 | 3 | -21.7% [-33.0, -9.2] |
| nim:z-ai/glm-5.3-flash | toonx | wtq | 22 | 3 | -24.6% [-30.4, -18.9] |
| nim:z-ai/glm-5.3-flash | toonx | wtq | 23 | 1 | -33.3% [-33.3, -33.3] |
| nim:z-ai/glm-5.3-flash | toonx | wtq | 24 | 4 | -20.0% [-25.7, -15.5] |
| nim:z-ai/glm-5.3-flash | toonx | wtq | 26 | 2 | -33.6% [-35.1, -30.7] |
| nim:z-ai/glm-5.3-flash | toonx | wtq | 27 | 5 | -26.6% [-32.9, -22.2] |
| nim:z-ai/glm-5.3-flash | toonx | wtq | 28 | 3 | -18.6% [-35.4, -12.5] |
| nim:z-ai/glm-5.3-flash | toonx | wtq | 29 | 2 | -21.6% [-21.7, -21.6] |
| nim:z-ai/glm-5.3-flash | toonx | wtq | 30 | 7 | -27.0% [-35.8, -18.6] |
| nim:z-ai/glm-5.3-flash | toonx | wtq | 31 | 1 | -14.6% [-14.6, -14.6] |
| nim:z-ai/glm-5.3-flash | toonx | wtq | 34 | 3 | -32.4% [-35.3, -26.6] |
| nim:z-ai/glm-5.3-flash | toonx | wtq | 35 | 4 | -44.1% [-47.8, -37.4] |
| nim:z-ai/glm-5.3-flash | toonx | wtq | 36 | 2 | -32.7% [-35.2, -28.1] |
| nim:z-ai/glm-5.3-flash | toonx | wtq | 38 | 1 | -38.3% [-38.3, -38.3] |
| nim:z-ai/glm-5.3-flash | toonx | wtq | 40 | 1 | -21.6% [-21.6, -21.6] |
| nim:z-ai/glm-5.3-flash | toonx | wtq | 42 | 2 | -22.7% [-26.5, -17.7] |
| nim:z-ai/glm-5.3-flash | toonx | wtq | 46 | 2 | -26.5% [-26.6, -26.4] |
| nim:z-ai/glm-5.3-flash | toonx | wtq | 50 | 2 | -21.5% [-21.5, -21.5] |
| nim:z-ai/glm-5.3-flash | toonx | wtq | 56 | 1 | -27.2% [-27.2, -27.2] |
| nim:z-ai/glm-5.3-flash | toonx | wtq | 60 | 2 | -17.3% [-24.6, -14.5] |
| nim:z-ai/glm-5.3-flash | toonx | wtq | 75 | 1 | -36.8% [-36.8, -36.8] |
| nim:z-ai/glm-5.3-flash | toonx | wtq | 95 | 1 | -51.8% [-51.8, -51.8] |
| nim:z-ai/glm-5.3-flash | toonx | wtq | 99 | 1 | -36.1% [-36.1, -36.1] |
| nim:z-ai/glm-5.3-flash | toonx | wtq | 111 | 2 | -41.6% [-41.6, -41.6] |
| nim:z-ai/glm-5.3-flash | toonx | wtq | 118 | 1 | -32.1% [-32.1, -32.1] |
| nim:z-ai/glm-5.3-flash | toonx | wtq | 176 | 1 | -43.2% [-43.2, -43.2] |
| nim:z-ai/glm-5.3-flash | toonx | wtq | 297 | 1 | -52.8% [-52.8, -52.8] |

### Calls

| model | format | calls | attempts | failed after retries (excluded) | model retired (excluded) | empty replies (scored incorrect) | served encoded by the gateway |
|---|---|---:|---:|---:|---:|---:|---:|
| nim:meta/llama-3.2-90b-vision-instruct | toonx | 44 | 44 | 0 | 0 | 0 | – |
| nim:meta/llama-3.2-90b-vision-instruct | json-compact | 44 | 44 | 1 | 0 | 0 | – |
| nim:meta/llama-3.2-90b-vision-instruct | json-compact-2 | 44 | 44 | 0 | 0 | 0 | – |
| nim:nvidia/nemotron-3-ultra-550b-a55b | json-compact | 100 | 100 | 0 | 0 | 0 | – |
| nim:nvidia/nemotron-3-ultra-550b-a55b | toonx | 100 | 100 | 0 | 0 | 0 | – |
| nim:nvidia/nemotron-3-ultra-550b-a55b | json-compact-2 | 100 | 100 | 0 | 0 | 0 | – |
| nim:openai/gpt-oss-20b | json-compact | 100 | 100 | 0 | 0 | 0 | – |
| nim:openai/gpt-oss-20b | json-compact-2 | 100 | 100 | 0 | 0 | 0 | – |
| nim:openai/gpt-oss-20b | toonx | 100 | 100 | 0 | 0 | 0 | – |
| nim:z-ai/glm-5.3-flash | json-compact-2 | 100 | 100 | 0 | 0 | 0 | – |
| nim:z-ai/glm-5.3-flash | toonx | 100 | 100 | 0 | 0 | 0 | – |
| nim:z-ai/glm-5.3-flash | json-compact | 100 | 100 | 0 | 0 | 0 | – |
