# End-to-end shadow evaluation run

**Run date:** 2026-10-02
**Model:** `openai/gpt-oss-20b` on NVIDIA NIM
**Raw pairs:** [`shadow-e2e-2026-10-02.pairs.jsonl`](shadow-e2e-2026-10-02.pairs.jsonl). They hold agreement and usage only, with no prompt or response text.

## Setup

One route was put in SHADOW with the default promotion thresholds (`NMin=200`, `NControlMin=50`, `ε=0.02`):

```json
{"routes":[{"tenant_id":"*","route_id":"shadow-e2e","version":"e2e.1","codec":"toon",
  "state":"SHADOW","shadow_sample_rate":1.0,"audit_sample_rate":0}]}
```

Production traffic was sent through the gateway with:

```sh
trimproof-gateway -policies policies.json -require-identity=false -eval-rps 0.4
bench drive -gateway http://127.0.0.1:8080/openai/v1 -route shadow-e2e \
  -n 400 -rpm 10 -seeds 6 -until-encoded 5
```

The traffic was the Phase 0 question sets (orders, logs, search and employees at 30 and 120 rows) over 6 data seeds. Each request carried its data as a `role: tool` result.

## Result: the route stayed in SHADOW (not promoted)

| | value |
|---|---|
| production requests | 400 sent, 0 failed, 345 correct (86%) |
| shadow pairs | 331 (3 invalid: an upstream error on one arm) |
| treatment pairs (JSON vs TOON) | 242, agreement **0.831** |
| control pairs (JSON vs JSON) | 88, agreement **0.864** (the noise floor) |
| Tier 1 discordance | none (the route has no validator) |
| measured input-token savings | **26.7%** (provider-reported usage) |
| decision | `SHADOW: agreement 0.830 below noise floor 0.862 − ε` |

Requests whose data failed Gate 4 (mostly search results, where TOON saves less than the route's 15% minimum) were forwarded unchanged and produced no pairs.

Agreement as pairs accumulated:

| valid pairs | treatment n | control n | treatment agreement | control agreement |
|---:|---:|---:|---:|---:|
| 50 | 33 | 17 | 0.848 | 0.882 |
| 100 | 69 | 31 | 0.826 | 0.935 |
| 150 | 110 | 40 | 0.791 | 0.925 |
| 200 | 147 | 53 | 0.796 | 0.887 |
| 300 | 222 | 78 | 0.820 | 0.872 |
| 330 | 242 | 88 | 0.831 | 0.864 |

## What this verifies

- **The full loop works against a real provider.** It covers production pass-through in SHADOW (no response was served encoded), sampling, concurrent paired arms within an evaluation rate budget, the comparator, stats, and the promotion rule.
- **The promoter reaches a decision at `NMin`** and holds the route with a stated reason.
- **Savings measured from provider usage (26.7%) match the Phase 0 estimate** of about 26% on gpt-oss.
- **Restart:**
  - The gateway was restarted on the same files and resumed from the 331 stored pairs.
  - New pairs were appended and the decision was re-evaluated over all of them.
  - No promotion happened, so the transition log stayed empty. Transition replay is therefore covered only by `TestReplayTransitions`, not by this run.

## What it shows about the promotion rule

The hold is a **point-estimate decision**: 0.831 < 0.864 − 0.02. The gap (3.3pp) is well inside sampling error. With 88 control pairs, the noise-floor estimate alone has a standard error of about ±3.7pp, so a gap this size cannot tell a TOON effect from noise.

The noise floor also drifted from 0.935 to 0.864 as data accumulated. In Phase 0, JSON-vs-TOON agreement on the same model equalled the noise floor (0.913 vs 0.913, n=46).

The current rule therefore:

- **can hold a route that is actually fine**, as here;
- **can promote by chance** when the point estimates happen to land the other way.

**Update:** this rule has since been replaced, so this run reflects the old rule.

The replacement is a three-arm paired non-inferiority test, decided at scheduled looks. It also stops early for routes that are clearly worse, and weights savings by output-token cost. See [PROMOTION.md](PROMOTION.md).

On this route's numbers, the 3.3pp gap is smaller than its uncertainty (about ±4pp), so the bounds straddle −ε. The new rule would therefore keep collecting rather than hold the route on a point estimate.
