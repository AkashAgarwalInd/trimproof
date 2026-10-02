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
