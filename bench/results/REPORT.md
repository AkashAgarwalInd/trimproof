# Phase 0 benchmark: verdict

**Run date:** 2026-10-02
**Raw data:** [`phase0-2026-10-02.jsonl`](phase0-2026-10-02.jsonl)
**Generated tables:** [`phase0-2026-10-02.md`](phase0-2026-10-02.md)

To regenerate the tables, run:

```sh
go run ./cmd/bench report -in bench/results/phase0-2026-10-02.jsonl
```

## Verdict: GO for TOON as the default codec

The go criterion is a ≥20% input-token reduction with accuracy within noise. TOON meets it on both models that have usable sample sizes.

| model | TOON input tokens vs compact JSON | Δ accuracy | noise floor (JSON vs JSON) | verdict |
|---|---:|---:|---:|---|
| openai/gpt-oss-20b | **−26.1%** | +0.0pp (1 vs 1 discordant) | 6.5% of answers flip | **GO** |
| nvidia/nemotron-3-super-120b-a12b | **−23.4%** | +0.0pp (2 vs 2 discordant) | 2.2% of answers flip | **GO** |
| moonshotai/kimi-k3 | −21.3% | n=9, too small | — | partial, excluded |

- The live savings match the offline o200k estimates: −26.1% vs compact JSON and −49% vs pretty JSON across all datasets.
- The tokenizer-based gate estimates can be trusted for the Gate 4 decision.
- The savings depend on the shape of the data:
  - employees, a wide uniform table: −36 to −41%;
  - orders and logs: −20 to −26%;
  - search results, which are long free-text snippets: only −12 to −15%.
- Gate 4's net-savings check exists for the search case: such payloads should often stay JSON.

## The other formats depend on the model

| model | tabular | csv |
|---|---|---|
| gpt-oss-20b | −26.5% tokens, +2.2pp: GO | −30.1% tokens, ±0pp: GO |
| nemotron-3-super | −23.3% tokens, **−8.7pp** (5 vs 1 discordant, p=0.22): **HOLD** | −26.9% tokens, −4.3pp: **HOLD** |

- The nemotron tabular drop is not statistically significant at n=46, but it is 4× the model's noise floor.
- The same format is neutral on gpt-oss.
- This is exactly the case for trimproof's core design. Whether a format is safe depends on the model and the route. It cannot be decided once, globally, so non-default codecs are enabled only through per-route shadow evaluation and promotion.
- CSV is not offered as a gateway codec, because it cannot preserve JSON types losslessly. It is benchmarked only as a lower bound on tokens.

## What this benchmark proves, and what it does not

**It proves:**
- TOON and tabular reduce the provider-billed input tokens on tabular data by 20–30% versus compact JSON. That is a real reduction, not a tokenizer artifact.
- The savings hold across two architecturally different models.

**It does not prove:**
- That accuracy is equivalent to within 2pp. With 46 paired questions per format per model, a difference of 2–5pp is undetectable: the noise floor alone is 2–6.5%. Detecting it needs hundreds of samples per route, which is what the shadow evaluator collects (see [PROMOTION.md](PROMOTION.md)).
- That the results generalize to Claude or GPT-4-class models. Only NVIDIA NIM-hosted open models were run, because no Anthropic key was available. A Claude run remains open (see Phase 7).
- That the results generalize to real traffic. The datasets are synthetic and seeded (orders, logs, search, employees), and the questions are exact-answer lookup, count, argmax and sum.

## Run notes

- **Exclusions:**
  - 83 of 598 calls failed and are excluded: 66 Kimi, 10 gpt-oss, 7 nemotron.
  - None of the failures are format-related: 47 were HTTP 429 rate limits, 29 were network or timeout errors, and 7 were calls cancelled when the run was stopped.
- **Kimi:**
  - dropped mid-run because of intermittent empty replies and rate limits;
  - its partial data is kept for transparency, but no verdict is drawn from it.
- **Empty replies:**
  - an empty reply after 3 attempts is scored as incorrect;
  - such replies were spread roughly evenly across formats (3–5 per format).
- **Weak question kinds:**
  - argmax and count questions are where every format loses accuracy;
  - lookups are 100% in all formats.
  - Future benchmarks should weight aggregation-style questions more heavily.

## Effect on the promotion defaults

- The measured JSON-vs-JSON flip rate of 2–6.5% confirms that the noise floor must be measured per route, not assumed.
- At n=46, a real −8.7pp signal could not reach significance, so a route needs hundreds of samples.
- These results led to the current rule, described in [PROMOTION.md](PROMOTION.md):
  - each sample runs JSON, codec and JSON on the same request;
  - a paired non-inferiority test with ε=0.02 is applied at scheduled looks;
  - savings are cost-weighted to include output tokens.
- Each Phase 0 question was answered in all three formats, so this file is also the data the design was simulated on.
