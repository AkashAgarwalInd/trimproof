# TOON cut our input tokens by a quarter. We still wouldn't switch it on blindly.

**Date:** 2026-10-02

Compact formats like [TOON](https://github.com/toon-format/toon) promise to cut the tokens an LLM spends reading tabular data: tool results, database rows, log lines, search hits. trimproof is a gateway that applies them. Before building it, we measured what such formats actually do to real model calls. This is what we found, and how it shaped the gateway.

**Summary:**
- **The input savings are real.** TOON cut provider-billed input tokens by 23–26% against compact JSON, and the offline token estimates matched the bills.
- **No accuracy change was detected**, but 46 questions per model can't rule out a 2–5pp effect. Sending the *same* JSON twice already flips 2–6.5% of answers.
- **The same format behaved differently on different models.** A strict tabular format was neutral on one model and lost 8.7pp on another.
- **The models wrote more.** Output tokens rose 13–36% with TOON. Priced at 4× input, that turns a 23–26% input saving into a 4–11% net saving.

So whether a format pays off depends on the model, the data and the task. trimproof therefore doesn't decide it once, globally. It measures each route on that route's own traffic, against that model's own noise floor, and counts output tokens too. The data and the code to reproduce everything here are in this repository.

## What was tested

- **Data:** four synthetic, seeded datasets at 30 and 120 rows:
  - orders;
  - logs;
  - search results with long text snippets;
  - a wide employee table.
- **Questions:** 46 exact-answer questions per format, of four kinds: lookup, count, argmax and sum. Each was asked as a user question with the data in a tool result.
- **Formats:**
  - compact JSON (the baseline);
  - TOON;
  - a strict lossless tabular codec;
  - CSV, as a lower bound only, since it can't preserve JSON types;
  - compact JSON a second time, to measure the noise floor.
- **Models:** `openai/gpt-oss-20b` and `nvidia/nemotron-3-super-120b-a12b` on NVIDIA NIM. `moonshotai/kimi-k3` was dropped mid-run because of rate limits and empty replies; its partial data is kept but not used.
- **Costs counted:** input tokens are the provider's own `usage` figures, and every format pays for its explanatory primer.

Full tables: [`bench/results/phase0-2026-10-02.md`](../bench/results/phase0-2026-10-02.md). Verdict and caveats: [`bench/results/REPORT.md`](../bench/results/REPORT.md).

## 1. The input savings are real, and depend on the data's shape

| model | TOON input tokens vs compact JSON |
|---|---:|
| gpt-oss-20b | **−26.1%** |
| nemotron-3-super | **−23.4%** |

The provider-reported savings matched the offline o200k counts (−26.1%), so a gateway can trust its own estimate when deciding whether encoding is worth it.

The shape of the data matters more than the model:

| dataset | TOON vs compact JSON |
|---|---:|
| employees (wide, uniform) | −36 to −41% |
| orders, logs | −20 to −26% |
| search (long free-text snippets) | −12 to −15% |

Against *pretty-printed* JSON, the savings are about −49%, nearly double. Check the baseline before comparing headline numbers: much of that difference is whitespace, which plain compact JSON already removes.

## 2. "No accuracy change" needs a noise floor to mean anything

On TOON, both models answered exactly as well as on JSON:
- gpt-oss: 1 question right only with JSON, and 1 right only with TOON;
- nemotron: 2 and 2.

That does not prove the formats are equivalent. The control arm sent the identical JSON request twice, and the answers already differed:

| model | answers that flip on an identical resend |
|---|---:|
| gpt-oss-20b | 6.5% |
| nemotron-3-super | 2.2% |

With noise of that size and 46 questions, a real 2–5pp drop is invisible. Detecting it takes hundreds of paired samples per route.

## 3. Same format, different model, different answer

| model | strict tabular codec | Δ accuracy |
|---|---:|---:|
| gpt-oss-20b | −26.5% tokens | +2.2pp |
| nemotron-3-super | −23.3% tokens | **−8.7pp** (5 vs 1 discordant) |

The nemotron drop is not statistically significant at this sample size (p=0.22), but it is four times that model's noise floor. The same format was neutral on gpt-oss.

There is no model-independent answer to "is this format safe?". The answer has to be measured where the format is used.

## 4. The models wrote more

This was the result we didn't expect. With TOON input, both reasoning models produced more output:

| model | input saved | output tokens | net saving with output priced 4× input |
|---|---:|---:|---:|
| gpt-oss-20b | 26% | +13% | **11%** |
| nemotron-3-super | 23% | +36% | **4%** |

The median TOON call on NIM was also about 10% slower, because decoding dominates latency. NIM's queueing makes latency noisy, but the extra output is real.

Any tool that reports only input-token savings overstates what you save, sometimes by most of it.

## What trimproof does about it

trimproof sits between your application and the Anthropic or OpenAI-compatible API. It re-encodes eligible data blocks, and it switches a route over only after measuring that route.

- **Lossless by construction.** Every codec must satisfy `Decode(Encode(x)) == canonical(x)` byte for byte, which fuzz tests enforce. Requests that don't qualify are forwarded byte-identical.
- **Three arms per sample.** In `SHADOW`, a sample of requests is replayed in the background three ways: as JSON, in the codec, and as JSON again. The second JSON arm is that request's own noise floor, which also cancels out how hard each question is.
- **A real statistical test.** Decisions are made only at scheduled looks, using a paired non-inferiority test (ε = 2pp). Re-checking a point estimate after every sample turned out to promote a route that is 5pp worse 55% of the time.

  Simulated on the Phase 0 answers ([details](../bench/results/PROMOTION.md)):

  | route | outcome |
  |---|---|
  | fine | promoted 98% of the time, median ~3,300 evaluation calls |
  | exactly 2pp worse | wrongly promoted 6% of the time |
  | 5pp worse | never promoted, and switched off after ~2,700 calls |
  | 10pp worse | switched off after ~600 calls |

- **Savings are cost-weighted.** Output tokens count at the route's `output_price_ratio` (default 4). A route is promoted only if its *net* savings reach the route's minimum (default 15%). On the Phase 0 numbers, neither model would be promoted. That is the intended outcome.
- **It keeps watching.** Enabled routes keep being sampled, and they are demoted on regression. With an optional Tier 1 validator (JSON Schema, exact-decimal business rules, authorization checks on tool calls), a circuit breaker also falls back to JSON when production failures rise.

The cost to production requests is small:

| payload | gateway time (p50) |
|---|---:|
| 10 KB | 1.4 ms |
| 100 KB | 13 ms |

Most of that time is spent counting tokens exactly. The gateway uses its own o200k counter, about 17× faster than tiktoken-go, and CI checks it against tiktoken-go for identical counts. See [`OVERHEAD.md`](../bench/results/OVERHEAD.md).

## An end-to-end run

One route was put in `SHADOW` on gpt-oss-20b and driven with 400 requests ([details](../bench/results/SHADOW-E2E.md)):
- **Measured input savings:** 26.7%, from provider usage, matching the benchmark.
- **The earlier promotion rule held the route** on a 3.3pp point-estimate gap: 0.831 codec agreement against a 0.864 noise floor. That gap was smaller than its own uncertainty (about ±4pp).
- **The rule was replaced** with the three-arm test above, which would keep collecting rather than decide on that evidence.

## What this does not show yet

- **No Claude or GPT-4-class models** were run, only open models hosted on NVIDIA NIM. The gateway supports Anthropic, and it calibrates its token estimates for Claude with `count_tokens`. A Claude benchmark run is the most important missing result.
- **Synthetic data and short answers.** Real tool results and longer answers may behave differently; that is exactly why the gateway measures each route.
- **Small samples.** At 46 questions per model, the accuracy results are consistent with "no change", not proof of it.
- **The rule's error rates are simulated,** from bootstrapped benchmark answers. A production-length shadow run has not yet reached a promotion decision.

trimproof is pre-alpha.

## Reproduce it

```sh
go run ./cmd/bench tokens                                       # offline token table, no API key
go run ./cmd/bench report -in bench/results/phase0-2026-10-02.jsonl
go run ./cmd/bench run -targets openai:MODEL,anthropic:MODEL    # live, resumable
go test ./pkg/eval -run ErrorRates -v                           # promotion rule error rates
```

Results from other models are welcome; open an issue with your `bench report` output.
