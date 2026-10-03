# Free-form check, 2026-10-03

Pre-registered in [Amendment 3](../VERIFY-PLAN.md#amendment-3-free-form-check-before-any-free-form-call); run with toonx 3 and 16,384 output tokens, as amended after it.
- **Records:** [freeform.jsonl](freeform.jsonl) (324 answers) and [freeform-judged.jsonl](freeform-judged.jsonl) (107 grades by `moonshotai/kimi-k3`).
- **Skipped:** one glm toonx answer (`freeform-itunes-search-jazz-compare`) hit the 12-minute call timeout. It was skipped, not retried, so that task is not graded.

Reproduce: `go run ./cmd/bench freeform-report -in bench/results/verify-2026-10/freeform.jsonl -judged bench/results/verify-2026-10/freeform-judged.jsonl`

Each task was answered with json-compact, json-compact-2 (the same JSON again: the noise floor) and toonx, then graded once by the judge with the answers under shuffled labels, against the data as JSON. Intervals are 95% percentile bootstrap over tasks (2000 resamples, seed 1).

## Quality by arm

| model | tasks | answers with an error: json / json-2 / toonx | toonx − json | json-2 − json | complete (1–5): json / json-2 / toonx |
|---|---:|---|---|---|---|
| nim:nvidia/nemotron-3-super-120b-a12b | 35 | 34% / 37% / 34% | +0.0pp [-17.1, +20.0] | +2.9pp [-14.3, +20.0] | 4.71 / 4.57 / 4.77 |
| nim:openai/gpt-oss-20b | 36 | 67% / 56% / 61% | -5.6pp [-16.7, +5.6] | -11.1pp [-30.6, +8.3] | 4.33 / 4.39 / 3.94 |
| nim:z-ai/glm-5.3-flash | 33 | 9% / 18% / 33% | +24.2pp [+6.1, +42.4] | +9.1pp [-3.0, +21.2] | 4.91 / 4.82 / 4.76 |
| **all models** | 104 | 38% / 38% / 43% | +5.8pp [-3.8, +15.4] | +0.0pp [-10.6, +9.6] | 4.64 / 4.59 / 4.48 |

**Pre-registered rule** (all models): upper bound of toonx − json +15.4pp against a 15pp margin: a drop larger than the margin is not ruled out.

## Do the answers agree?

"Same facts" is the judge's call on each pair. Word overlap is the F1 of the two replies' words (1 = same words). json vs json-2 is the model's own variation on identical input.

| model | same facts: json vs toonx | same facts: json vs json-2 | word overlap (median): json vs toonx | json vs json-2 |
|---|---|---|---:|---:|
| nim:nvidia/nemotron-3-super-120b-a12b | 26% [11, 40] | 43% [26, 60] | 0.57 | 0.69 |
| nim:openai/gpt-oss-20b | 14% [3, 25] | 33% [19, 50] | 0.48 | 0.67 |
| nim:z-ai/glm-5.3-flash | 48% [30, 67] | 73% [58, 88] | 0.68 | 0.75 |
| **all models** | 29% [20, 38] | 49% [40, 60] | 0.60 | 0.70 |

## Tokens

| model | input saved by toonx | output tokens: json / toonx (mean) |
|---|---:|---|
| nim:nvidia/nemotron-3-super-120b-a12b | 25.2% | 1352 / 2044 |
| nim:openai/gpt-oss-20b | 27.3% | 733 / 756 |
| nim:z-ai/glm-5.3-flash | 26.9% | 986 / 1130 |

## Calls

- json-compact: 108 calls, 0 failed, 0 empty replies (graded as given).
- json-compact-2: 108 calls, 0 failed, 0 empty replies (graded as given).
- toonx: 108 calls, 1 failed, 0 empty replies (graded as given).
- judge: 107 records, 3 not usable (excluded).
