# Real API payloads through trimproof's gates

Fetched 2026-10-02 13:27 UTC from 23 public, keyless endpoints; 19 stored, 4 failed: spacex/rockets (HTTP 525 ); googlebooks/search-golang (HTTP 429 Too Many Requests); datausa/state-population (HTTP 404 Not Found); mastodon/public-timeline (HTTP 422 Unprocessable Entity).

Each response body is judged by `policy.Decide` exactly as sent, as one tool result, under the default route policy with the codec ENABLED: strict schema, minimum 200 tokens, minimum net savings 15% with the primer counted. Token counts are o200k. Net savings are (JSON − encoded − primer) / JSON against compact canonical JSON.

## Overall

| | toonx | toon | tabular |
|---|---:|---:|---:|
| payloads eligible | 8 / 19 (42%) | 1 / 19 (5%) | 0 / 19 (0%) |
| median net savings, eligible payloads | 24.8% | 25.4% | — |
| token-weighted net savings, eligible payloads | 42.4% | 25.4% | — |
| token-weighted net savings, all payloads (ineligible ones save 0) | 28.0% | 0.1% | 0.0% |
| median savings vs pretty-printed JSON, eligible payloads | 45.6% | 47.7% | — |

## By API

| API | payloads | toonx eligible | toon eligible | tabular eligible |
|---|---:|---:|---:|---:|
| github | 2 | 1 | 0 | 0 |
| gitlab | 1 | 1 | 0 | 0 |
| jsonplaceholder | 2 | 0 | 0 | 0 |
| restcountries | 1 | 0 | 0 | 0 |
| rickandmorty | 1 | 0 | 0 | 0 |
| wikipedia | 1 | 1 | 1 | 0 |
| crates | 1 | 1 | 0 | 0 |
| dockerhub | 1 | 1 | 0 | 0 |
| huggingface | 1 | 0 | 0 | 0 |
| er-api | 1 | 0 | 0 | 0 |
| chicago-data | 1 | 1 | 0 | 0 |
| itunes | 1 | 1 | 0 | 0 |
| musicbrainz | 1 | 0 | 0 | 0 |
| dictionaryapi | 1 | 0 | 0 | 0 |
| openalex | 1 | 0 | 0 | 0 |
| citibike | 1 | 1 | 0 | 0 |
| kraken | 1 | 0 | 0 | 0 |

## Per payload

Each codec cell is the net saving when the payload is eligible (**bold**), else the net saving where encoding was attempted, then the rejecting gate.

| API | payload | KB | JSON tok | toonx | toon | tabular |
|---|---|---:|---:|---|---|---|
| github | k8s-issue-comments | 51.2 | 14989 | **42.5%** | -12.5% · net-savings: encoding is not smaller | structural: ineligible: row 0 key "reactions" holds a nested container |
| github | torvalds-events | 26.8 | 8752 | -20.7% · net-savings: encoding is not smaller | -14.8% · net-savings: encoding is not smaller | structural: ineligible: row 0 key "actor" holds a nested container |
| gitlab | projects | 28.3 | 9045 | **22.3%** | -14.3% · net-savings: encoding is not smaller | structural: ineligible: row 0 key "namespace" holds a nested container |
| jsonplaceholder | users | 5.5 | 1234 | 8.6% · net-savings: net savings 8.6% < 15.0% | -21.4% · net-savings: encoding is not smaller | structural: ineligible: row 0 key "address" holds a nested container |
| jsonplaceholder | comments | 154.0 | 35760 | 12.6% · net-savings: net savings 12.6% < 15.0% | 12.6% · net-savings: net savings 12.6% < 15.0% | 13.8% · net-savings: net savings 13.8% < 15.0% |
| restcountries | europe | 0.3 | 48 | min-size: 48 < 200 tokens | min-size: 48 < 200 tokens | structural: ineligible: top-level value is not an array |
| rickandmorty | characters | 19.0 | 5898 | 11.8% · net-savings: net savings 11.8% < 15.0% | -9.0% · net-savings: encoding is not smaller | structural: ineligible: top-level value is not an array |
| wikipedia | recent-changes | 8.9 | 3333 | **25.4%** | **25.4%** | structural: ineligible: top-level value is not an array |
| crates | top-downloads | 46.0 | 14065 | **30.9%** | -14.5% · net-savings: encoding is not smaller | structural: ineligible: top-level value is not an array |
| dockerhub | library-repos | 36.5 | 9939 | **24.1%** | structural: ineligible: toon-go round trip failed | structural: ineligible: top-level value is not an array |
| huggingface | top-models | 28.9 | 9770 | 4.6% · net-savings: net savings 4.6% < 15.0% | -6.3% · net-savings: encoding is not smaller | structural: ineligible: row 0 key "tags" holds a nested container |
| er-api | usd-rates | 2.9 | 1415 | -30.0% · net-savings: encoding is not smaller | -29.9% · net-savings: encoding is not smaller | structural: ineligible: top-level value is not an array |
| chicago-data | crimes | 62.0 | 20051 | **21.5%** | -18.2% · net-savings: encoding is not smaller | structural: ineligible: row 0 key "location" holds a nested container |
| itunes | search-jazz | 106.0 | 33114 | **17.6%** | structural: ineligible: number 29.90 does not survive float64 formatting | structural: ineligible: top-level value is not an array |
| musicbrainz | releases | 54.2 | 21997 | -10.1% · net-savings: encoding is not smaller | structural: ineligible: toon-go round trip failed | structural: ineligible: top-level value is not an array |
| dictionaryapi | run | 8.8 | 2090 | -4.8% · net-savings: encoding is not smaller | structural: ineligible: toon-go round trip failed | structural: ineligible: row 0 key "license" holds a nested container |
| openalex | works-transformers | 566.6 | 161309 | -26.7% · net-savings: encoding is not smaller | structural: ineligible: number 1.0 does not survive float64 formatting | structural: ineligible: top-level value is not an array |
| citibike | stations | 1332.2 | 439086 | **46.5%** | structural: ineligible: toon-go round trip failed | structural: ineligible: top-level value is not an array |
| kraken | trades | 67.2 | 30772 | -6.9% · net-savings: encoding is not smaller | -10.1% · net-savings: encoding is not smaller | structural: ineligible: top-level value is not an array |
