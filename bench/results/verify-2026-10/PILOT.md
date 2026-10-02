# Pilot, 2026-10-02

This is the pilot pre-registered in [Amendment 2](../VERIFY-PLAN.md#amendment-2-pilot-before-any-result). It checks that the run works; it is not a result. Every call went to NIM's free tier.
- **Records:** [pilot.jsonl](pilot.jsonl) and [smoke.jsonl](smoke.jsonl).
- **Manifests:** [pilot-payloads.manifest.json](pilot-payloads.manifest.json) and [pilot-wtq.manifest.json](pilot-wtq.manifest.json).

## Smoke checks (5 calls per candidate)

| slot | candidate | outcome |
|---|---|---|
| OpenAI open weights | `openai/gpt-oss-20b` | passed (earlier smoke check) |
| NVIDIA | `nvidia/nemotron-3-super-120b-a12b` | passed (earlier smoke check) |
| Mistral | `mistralai/mistral-large-2-instruct`, `mistralai/mistral-large`, `nv-mistralai/mistral-nemo-12b-instruct` | all HTTP 404: not available to this account |
| Google | `google/gemma-3-12b-it` | HTTP 404 |
| Google | `google/gemma-4-31b-it` | 5 of 5 timed out after 6 minutes |
| Google | `google/gemma-3-4b-it` | HTTP 404 |
| DeepSeek | `deepseek-ai/deepseek-v4.1-flash` | 1 of 5 answered (in 252 s); 4 timed out |
| DeepSeek | `z-ai/glm-5.3-flash` | passed: 5 of 5 correct, about 110 s each |

Under the smoke rule, the Mistral and Google slots have no model, and `z-ai/glm-5.3-flash` takes the DeepSeek slot.

## Pilot calls

| run | model | format | correct | input tokens | output tokens | truncated at 4096 |
|---|---|---|---:|---:|---:|---:|
| payload Q&A | gpt-oss-20b | json-compact | 4/5 | 26,991 | 2,298 | 0 |
| payload Q&A | gpt-oss-20b | toonx | 4/5 | 21,258 | 2,200 | 0 |
| payload Q&A | nemotron-3-super | json-compact | 4/5 | 33,400 | 2,330 | 0 |
| payload Q&A | nemotron-3-super | toonx | 3/5 | 27,219 | 10,386 | 1 |
| payload Q&A | glm-5.3-flash | json-compact | 5/5 | 27,503 | 801 | 0 |
| payload Q&A | glm-5.3-flash | toonx | 5/5 | 21,999 | 1,603 | 0 |
| synthetic | gpt-oss-20b | json-compact / -2 / toonx / toonx-p2 | 4/4, 3/3, 3/3, 2/3 | | | 0 |
| WikiTableQuestions | gpt-oss-20b | json-compact / toonx | 3/3, 3/3 | 4,484 / 3,301 | | 0 |
| gateway path | gpt-oss-20b | gateway | 3/4 | | | 0 |

**Checks passed:**
- No HTTP errors in the pilot.
- Every reply was scored.
- The gateway served toonx on all 4 requests and recorded `est_savings` (0.21–0.37).
- Its input tokens match the direct toonx calls.

**Payload Q&A:**
- **Input:** toonx used 19.8% fewer input tokens over the 15 pairs.
- **Output:** toonx used more output tokens, mostly from one nemotron reply that hit the 4096-token cap with no answer (`tvmaze-shows-q4`), which was scored incorrect as pre-registered.
- **Errors:** the only answer gpt-oss missed in both formats was a count. Nemotron missed that count in both formats too.

With 5 questions per arm, none of these numbers support a claim.

## Calls

- **Smoke checks today:** 30 calls.
- **Pilot:** 53 records. The empty nemotron reply may have been retried up to twice under the registered retry rule.
- **Earlier smoke check:** 20 calls, disclosed in Amendment 1.

# Pilot 2 and its diagnostic, 2026-10-02

Pre-registered in [Amendment 4](../VERIFY-PLAN.md#amendment-4-toonx-2-before-any-result) and [Amendment 5](../VERIFY-PLAN.md#amendment-5-pilot-2-failed-its-rule-diagnostic-before-any-result). These runs check the run itself; they are not results.

## Pilot 2: toonx 2 on its new forms ([pilot2.jsonl](pilot2.jsonl))

| model | JSON correct | toonx 2 correct | input saved | input + output saved |
|---|---:|---:|---:|---:|
| gpt-oss-20b | 5/6 | 5/6 | 38.8% | 33.7% |
| nemotron-3-super | 6/6 | 4/6 | 35.8% | 19.5% |
| glm-5.3-flash | 6/6 | 6/6 | 38.4% | 35.9% |
| **all** | **17/18** | **15/18** | **37.6%** | **29.0%** |

- **Lookups that needed a new form:** every model answered correctly, 12 of 12. That covers rebuilding a URL from its prefix and reading a field from the `all rows` line.
- **Both toonx misses:** nemotron used all 4,096 output tokens and gave no answer, on a count and on a "largest value" question.
- **Amendment 4's rule fired:** toonx was 2 short of JSON, against an allowed 1.

## Diagnostic: nemotron-3-super, 3 failing questions × 3 repeats ([diag/](diag/))

| output limit | format | correct | cut off at the limit | mean output tokens |
|---:|---|---:|---:|---:|
| 4,096 | json-compact | 9/9 | 0 | 738 |
| 4,096 | toonx 1 (`toonx1`) | 1/9 | 8 | 4,072 |
| 4,096 | toonx 2 | 0/9 | 9 | 4,096 |
| 16,384 | toonx 1 (`toonx1`) | 9/9 | 0 | 10,341 |
| 16,384 | toonx 2 | 9/9 | 0 | 10,486 |

**Outcome under Amendment 5's fixed reading:**
- **The forms are not the cause.** toonx 1 failed as often as toonx 2: 1 more failure for toonx 2, against a threshold of 3. **toonx 2 is kept.** The write-up must say that pilot 2's rule fired and why it was set aside.
- **The output limit is the cause.** At 16,384 both versions answered 9 of 9. **The full run uses max_tokens 16,384 for every model and arm.**

**Notes:**
- **Timeouts:** 3 calls at 16,384 hit the harness's 5-minute per-call timeout and were retried once by the normal resume. All 3 succeeded. As a harness fix, the per-call timeout is now 12 minutes.
- **Finding to report:** on these count and largest-value questions nemotron writes about 14× more output tokens on toonx (about 10,400) than on JSON (738). Its answers are right when it has room. Over both pilots, the other models write 2–4× more output on toonx lookups (about +250 tokens each) and about the same on counts.
- **Manifests:** the two runs per output file overwrote each other's `.manifest.json`, so those files are not kept. The question IDs are listed in Amendment 5.

**Calls:**
- Pilot 2: 36 calls.
- Diagnostic: 45 calls plus 3 retries.

# Pilot 3: split wide tables, 2026-10-02

Pre-registered in [Amendment 6](../VERIFY-PLAN.md#amendment-6-split-wide-tables-before-any-result). It checks the run itself; it is not a result.
- **Records:** [pilot3.jsonl](pilot3.jsonl) and [pilot3.manifest.json](pilot3.manifest.json).
- **Setup:** 10 questions × 3 arms × 3 models, at 16,384 output tokens. 90 calls, 0 errors.
- **Cost** is input + 4 × output tokens.

| model | arm | correct | input saved | output tokens | cost saved |
|---|---|---:|---:|---:|---:|
| gpt-oss-20b | json-compact | 9/10 | | 1,559 | |
| | toonx | 7/10 | 38.6% | 9,375 | 8.5% |
| | **toonx-split** | **8/10** | 33.9% | 1,585 | **32.0%** |
| nemotron-3-super | json-compact | 9/10 | | 15,465 | |
| | toonx | 8/10 | 36.3% | 41,504 | −32.9% |
| | **toonx-split** | **10/10** | 31.8% | 8,493 | **36.3%** |
| glm-5.3-flash | json-compact | 10/10 | | 2,445 | |
| | toonx | 10/10 | 38.3% | 2,296 | 35.6% |
| | **toonx-split** | **10/10** | 33.7% | 961 | **35.9%** |
| **pooled** | toonx | 25/30 vs 28/30 | | | **−2.2%** |
| **pooled** | toonx-split | 28/30 vs 28/30 | | | **35.0%** |

**Outcome under Amendment 6's fixed reading:**
- **toonx-split qualifies.**
  - Its pooled cost saving is 35.0%, against a minimum of 15%.
  - No model answers more than 1 fewer question correctly than JSON: gpt-oss 8 vs 9, nemotron 10 vs 9, glm 10 vs 10.
- **toonx does not qualify:** −2.2% pooled, and gpt-oss answers 7 vs 9.
- **So toonx-split becomes toonx version 3,** and the full run's toonx arm uses it.

**Notes:**
- **One outlier flatters nemotron.** Its JSON reply to `github-linux-commits-q3` used 9,057 output tokens. Without that question, toonx-split saves 30.8% pooled (nemotron 22.2%, gpt-oss 34.5%, glm 38.4%), and toonx −15.9%.
- **Sample size:** 10 questions per model. None of these numbers supports a claim.

**Calls:**
- Pilot 3: 90 calls.
- Reasoning capture before Amendment 6: 10 calls, in [reasoning/](reasoning/).
