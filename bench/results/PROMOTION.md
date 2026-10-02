# Promotion rule

**Date:** 2026-10-02

## The rule

**Sampling.** Each sampled request runs three arms at the same time, in the background after the response has been served:

| arm | sends |
|---|---|
| A | the request as JSON |
| B | the request in the codec |
| C | the request as JSON again |

A against C is that request's own noise floor. The per-sample difference is:

```
d = mean(agree(A,B), agree(C,B)) − agree(A,C)
```

**Decisions** happen only at scheduled looks: at 200 samples, then every 100 samples after that. At each look the bounds are `mean(d) ∓ 2.5·SE`.

| condition at a look | result |
|---|---|
| lower bound > −0.02 and measured savings ≥ the route's minimum | **ENABLED** |
| upper bound < −0.02 | **OFF**, rejected. Sampling stops. |
| 4,000 samples without a decision | **OFF**, inconclusive |
| anything else | keep collecting |

**ENABLED routes** keep being sampled. The same bounds are applied over the last 400 samples, and an upper bound below −0.02 demotes the route to SHADOW. The production Tier 1 circuit breaker and the McNemar check are unchanged.

**Evidence resets at every transition.** Samples from before a route's last state change are not reused. This also holds across restarts, through the replayed transition log.

**Measured savings are cost-weighted:**

```
1 − (in_codec + k·out_codec) / (in_json + k·out_json)
```

`k` is the route's `output_price_ratio` (default 4). A codec that makes the model write more is charged for it.

## Why the previous rule was replaced

The previous rule compared point estimates (treatment agreement < control agreement − ε) after every pair once 200 pairs existed. Re-checking after every pair means a held route eventually gets a lucky estimate.

Simulated with 0.86 agreement (the end-to-end run's noise floor), 25% control pairs, and 6,000 pairs per route:

| true agreement drop | routes promoted |
|---|---:|
| none | 100% |
| 3pp | 82% |
| 5pp | **55%** |
| 10pp | **12%** |

## Design comparison

This was simulated by bootstrapping the 46 Phase 0 gpt-oss questions, each answered as JSON, JSON and TOON. Each route had a budget of 12,000 evaluation calls.

| design | fine route promoted | median calls to promote | route exactly at ε promoted (target ≈5%) | route 5pp worse |
|---|---:|---:|---:|---|
| 2 arms, 50% control, check after every pair, z=1.645 | 95% | 1,870 | 28% | 1% promoted; sampling never stops |
| 2 arms, 50% control, doubling looks, z=2.2, futility stop | 49% | 3,300 | 4% | 0% promoted; 75% stopped |
| 3 arms, check after every sample, z=1.645 | 100% | 930 | 37% | 1% promoted |
| **3 arms, a look every 100 samples, z=2.5, futility stop (shipped)** | **98%** | **3,300** | **6%** | **0% promoted; stopped by ~2,700 calls** |

- **Checking after every sample** makes the error rate several times the nominal 5%, whatever the test.
- **Fixing that with 2 arms** leaves half of the fine routes unpromoted within the budget.
- **Three arms on the same request** cancel out how hard each question is. Measured on Phase 0, variance per upstream call drops 2.4× for gpt-oss and nemotron and 3.5× for kimi. That makes a valid schedule of looks affordable.
- **A route that is 10pp worse** is stopped after about 600 calls.

## CI check

`TestPromotionErrorRates` runs the shipped thresholds against a model where each question has a stability *q*: 75% of questions have *q* = 1 and 25% have *q* = 0.663, which gives a 0.86 noise floor. The test fails if these rates drift.

| true agreement drop | promoted | rejected (OFF) |
|---|---:|---:|
| none | 100% (median 700 samples, about 2,100 calls) | 0% |
| 2pp (= ε) | 6.8% | 2.5% |
| 5pp | 0% | 100% |
| 10pp | 0% | 100% |

This model has more variation between questions than the Phase 0 bootstrap, so it reaches decisions faster. Real routes should fall between the two.

## Latency and cost

- **Production latency is unchanged.** Arms run after the response is served, within the evaluation budget (`-eval-rps`, `-eval-max-inflight`).
- **One sample takes as long as its slowest arm.** On NIM, the median was 16 s for 3 arms vs 5.4 s for 2 arms on gpt-oss. Callers never see this.
- **Each sample costs 3 calls.** The in-flight limit was raised from 4 to 6 so that two whole samples can run in parallel.
- **Time to a decision** is bounded by the evaluation budget. On NIM it is about 3 hours for gpt-oss or nemotron at the default limits.

### Output tokens

Phase 0 showed that the codec changes how much reasoning models write:

| model | input tokens saved | output tokens | net saving at `output_price_ratio` 4 |
|---|---:|---:|---:|
| gpt-oss-20b | 26% | +13% | 11% |
| nemotron-120b | 23% | +36% | 4% |

Promotion now uses the net figure. With the default 15% minimum, neither model's route would be promoted on this data: the input savings alone would clear 15%, but the net savings do not. The 15% net default is kept on purpose: a route where the codec makes the model write enough extra to cancel most of the input saving should not be promoted. Phase 0 answers are short, so this will be revisited with shadow data from real routes. Shadow samples also record each arm's latency, and the promotion reason reports the codec's median latency ratio.

## To reproduce

```sh
go test ./pkg/eval -run ErrorRates -v
```

The design comparisons were one-off simulations over [`phase0-2026-10-02.jsonl`](phase0-2026-10-02.jsonl) and the end-to-end run's agreement rates. They are not part of the repository.

Legacy two-arm samples (`TREATMENT`/`CONTROL`) in older pair files still load, but they no longer count toward promotion.
