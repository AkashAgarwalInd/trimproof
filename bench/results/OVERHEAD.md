# Gateway latency overhead

**Run date:** 2026-10-02, Apple M2, Go 1.27.1

To reproduce:

```sh
go test ./pkg/server -run '^$' -bench GatewayOverhead -benchtime 1000x
```

The benchmark sends one Anthropic request through the full gateway path: parse, gates, encode, render, forward and respond. The upstream is answered in-process, so the numbers are the gateway's own cost. The request carries one tool result of order-like rows, and the route uses TOON.

| payload | route state | p50 | p99 | allocated per request |
|---|---|---:|---:|---:|
| 10 KB | OFF | 0.38 ms | 0.62 ms | 0.2 MB |
| 10 KB | SHADOW | 1.2 ms | 2.1 ms | 0.5 MB |
| 10 KB | ENABLED | 1.4 ms | 2.1 ms | 0.6 MB |
| 100 KB | OFF | 3.8 ms | 3.9 ms | 2.0 MB |
| 100 KB | SHADOW | 11.5 ms | 15–23 ms | 4.7 MB |
| 100 KB | ENABLED | 13.4 ms | 14.9 ms | 5.5 MB |

The SHADOW p99 varied between runs on a busy laptop.

## What changed in Phase 7

The first run measured **84 ms p50** at 100 KB and **8.6 ms** at 10 KB with TOON on. Almost all of that was token counting: the Gate 3 and Gate 4 estimates use tiktoken-go, whose regex-based pre-tokenizer runs at about 2.5 MB/s.

| o200k counter, 100 KB JSON | time | allocated |
|---|---:|---:|
| pkoukk/tiktoken-go (previous) | 41.6 ms | 17 MB |
| hupe1980/go-tiktoken | 41.4 ms | 20 MB |
| tiktoken-go/tokenizer | 14.0 ms | 9.5 MB |
| trimproof `pkg/tokens` (hand-written pre-tokenizer) | 2.4 ms | 0.1 MB |

- All four give identical counts.
- trimproof's counter is checked against tiktoken-go in CI by differential tests and a fuzz target, so exactness is enforced rather than assumed.
- The gates also stopped parsing and checking each block twice. The `codec.ValueEncoder` fast path is fuzz-checked to match `Encode` byte for byte.

## Where the remaining 100 KB time goes

These are approximate per-request costs:

| stage | time |
|---|---:|
| TOON encode plus the round-trip verification decode (toon-go) | ~3.8 ms |
| two exact token counts (JSON and TOON) | ~4 ms |
| canonical parse, marshal and check | ~2.3 ms |
| provider parse and re-render | ~3.4 ms |

At 100 KB, a tool result is about 30k tokens, and model calls take seconds. A ~13 ms gateway cost is therefore under 1% of request latency. In a 20-step agent it adds roughly 30–260 ms to a task that runs for minutes.

Fewer input tokens do not make calls faster end to end. In Phase 0 on NVIDIA NIM, TOON calls had a median latency about 10% higher than JSON calls. The models wrote more output tokens (gpt-oss +13%, nemotron +36%), and decoding dominates latency. NIM's queueing makes these latency figures noisy, but the extra output is real.

The [October verification run](verify-2026-10/VERIFY.md) on four models shows the same dependence on output. The median latency ratio of `toonx` to JSON ran from ×0.74 to ×1.28, depending on the model and the data set. The slowest cells were the ones where a reasoning model wrote more: nemotron-3-ultra was ×1.28 on synthetic tables, where its output rose 69%.

Shadow samples therefore record each arm's latency and output tokens, and promotion weights output tokens by cost (see [PROMOTION.md](PROMOTION.md)).

The original target of p99 < 5 ms at 100 KB is not met.

Further reductions would mean replacing toon-go's encoder and verifier or reusing the provider parse for the gates. That work is left until real traffic shows it matters.
