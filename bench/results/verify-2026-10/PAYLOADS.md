# Real API payloads through trimproof's gates

Fetched 2026-10-02 12:51 UTC from 40 public, keyless endpoints; 40 stored.

Each payload is judged by `policy.Decide`, as one tool result, under the default route policy with the codec ENABLED: strict schema, minimum 200 tokens, minimum net savings 15% with the primer counted. Token counts are o200k.

"As sent" is the response body unchanged. "Inner array" applies only to responses whose top level is an object: it is the largest array of objects inside, as sent by a client that unwraps the response first. The gateway itself never unwraps.

## Overall

| | toon | tabular |
|---|---:|---:|
| payloads eligible as sent | 3 / 40 (8%) | 3 / 40 (8%) |
| eligible as sent or via inner array | 3 / 40 (8%) | 3 / 40 (8%) |
| median net savings, eligible as sent | 28.9% | 27.7% |
| mean net savings, eligible as sent | 28.1% | 27.6% |
| token-weighted net savings, eligible as sent | 25.5% | 25.3% |
| median net savings, eligible as sent or via inner array | 28.9% | 27.7% |
| token-weighted net savings, eligible as sent or via inner array | 25.5% | 25.3% |
| median savings vs pretty-printed JSON, eligible as sent or via inner array | 47.1% | 46.2% |

## By API

| API | payloads | toon eligible | tabular eligible | toon eligible incl. inner array | median toon savings when eligible |
|---|---:|---:|---:|---:|---:|
| github | 12 | 2 | 2 | 2 | 25.7% |
| npm | 2 | 0 | 0 | 0 | — |
| pypi | 2 | 0 | 0 | 0 | — |
| hn | 2 | 0 | 0 | 0 | — |
| hn-algolia | 2 | 0 | 0 | 0 | — |
| open-meteo | 1 | 0 | 0 | 0 | — |
| usgs | 1 | 0 | 0 | 0 | — |
| nws | 1 | 0 | 0 | 0 | — |
| coingecko | 2 | 0 | 0 | 0 | — |
| frankfurter | 1 | 0 | 0 | 0 | — |
| stackexchange | 1 | 0 | 0 | 0 | — |
| openlibrary | 1 | 0 | 0 | 0 | — |
| crossref | 1 | 0 | 0 | 0 | — |
| nobelprize | 1 | 0 | 0 | 0 | — |
| nyc-opendata | 2 | 0 | 0 | 0 | — |
| tvmaze | 2 | 0 | 0 | 0 | — |
| pokeapi | 1 | 0 | 0 | 0 | — |
| artic | 1 | 0 | 0 | 0 | — |
| federalregister | 1 | 0 | 0 | 0 | — |
| carbonintensity | 1 | 0 | 0 | 0 | — |
| openbrewerydb | 1 | 1 | 1 | 1 | 33.0% |
| worldbank | 1 | 0 | 0 | 0 | — |

## Per payload, as sent

| API | payload | KB | toon | toon gate: reason | JSON tok | TOON tok | toon net | tabular | tabular gate: reason | tabular net |
|---|---|---:|---|---|---:|---:|---:|---|---|---:|
| github | go-issues | 242.2 | no | structural: ineligible: row 1 has 33 keys, want 34 | — | — | — | no | structural: ineligible: row 0 key "assignee" holds a nested container | — |
| github | rust-closed-issues | 375.8 | no | structural: ineligible: nested empty array | — | — | — | no | structural: ineligible: row 0 key "assignees" holds a nested container | — |
| github | kubernetes-pulls | 703.7 | no | structural: ineligible: nested empty array | — | — | — | no | structural: ineligible: row 0 key "_links" holds a nested container | — |
| github | cpython-closed-pulls | 533.5 | no | structural: ineligible: nested empty array | — | — | — | no | structural: ineligible: row 0 key "_links" holds a nested container | — |
| github | linux-commits | 123.6 | no | structural: ineligible: toon-go round trip failed | 41013 | — | — | no | structural: ineligible: row 0 key "author" holds a nested container | — |
| github | vscode-releases | 38.5 | no | structural: ineligible: nested empty array | — | — | — | no | structural: ineligible: row 0 key "assets" holds a nested container | — |
| github | react-contributors | 93.3 | **yes** | — | 25809 | 19919 | 22.5% | **yes** | — | 22.8% |
| github | node-tags | 39.2 | no | structural: ineligible: toon-go round trip failed | 16306 | — | — | no | structural: ineligible: row 0 key "commit" holds a nested container | — |
| github | go-labels | 25.9 | **yes** | — | 8048 | 5635 | 28.9% | **yes** | — | 27.7% |
| github | google-repos | 268.6 | no | structural: ineligible: nested empty array | — | — | — | no | structural: ineligible: row 0 key "custom_properties" holds a nested ... | — |
| github | go-workflow-runs | 355.9 | no | structural: ineligible: top-level value is not an array | — | — | — | no | structural: ineligible: top-level value is not an array | — |
| github | search-repos | 161.3 | no | structural: ineligible: top-level value is not an array | — | — | — | no | structural: ineligible: top-level value is not an array | — |
| npm | search-react | 52.0 | no | structural: ineligible: top-level value is not an array | — | — | — | no | structural: ineligible: top-level value is not an array | — |
| npm | search-cli | 52.1 | no | structural: ineligible: top-level value is not an array | — | — | — | no | structural: ineligible: top-level value is not an array | — |
| pypi | requests | 188.5 | no | structural: ineligible: top-level value is not an array | — | — | — | no | structural: ineligible: top-level value is not an array | — |
| pypi | flask | 86.7 | no | structural: ineligible: top-level value is not an array | — | — | — | no | structural: ineligible: top-level value is not an array | — |
| hn | topstories-ids | 4.4 | no | structural: ineligible: row 0 is not an object | — | — | — | no | structural: ineligible: row 0 is not an object | — |
| hn | item | 0.4 | no | structural: ineligible: top-level value is not an array | — | — | — | no | structural: ineligible: top-level value is not an array | — |
| hn-algolia | search-llm | 75.6 | no | structural: ineligible: top-level value is not an array | — | — | — | no | structural: ineligible: top-level value is not an array | — |
| hn-algolia | latest-stories | 40.9 | no | structural: ineligible: top-level value is not an array | — | — | — | no | structural: ineligible: top-level value is not an array | — |
| open-meteo | berlin-hourly | 2.5 | no | structural: ineligible: top-level value is not an array | — | — | — | no | structural: ineligible: top-level value is not an array | — |
| usgs | quakes-week-m2.5 | 225.7 | no | structural: ineligible: top-level value is not an array | — | — | — | no | structural: ineligible: top-level value is not an array | — |
| nws | alerts-ca | 103.0 | no | structural: ineligible: top-level value is not an array | — | — | — | no | structural: ineligible: top-level value is not an array | — |
| coingecko | markets | 38.2 | no | structural: ineligible: number 20092434.0 does not survive float64 fo... | — | — | — | no | structural: ineligible: row 1 key "roi" holds a nested container | — |
| coingecko | exchanges | 51.4 | no | net-savings: net savings 14.9% < 15.0% | 12041 | 10154 | 14.9% | no | net-savings: net savings 14.8% < 15.0% | 14.8% |
| frankfurter | rates-september | 1.2 | no | structural: ineligible: top-level value is not an array | — | — | — | no | structural: ineligible: top-level value is not an array | — |
| stackexchange | so-questions | 37.4 | no | structural: ineligible: top-level value is not an array | — | — | — | no | structural: ineligible: top-level value is not an array | — |
| openlibrary | search | 19.0 | no | structural: ineligible: top-level value is not an array | — | — | — | no | structural: ineligible: top-level value is not an array | — |
| crossref | works-llm | 178.6 | no | structural: ineligible: top-level value is not an array | — | — | — | no | structural: ineligible: top-level value is not an array | — |
| nobelprize | laureates | 186.7 | no | structural: ineligible: top-level value is not an array | — | — | — | no | structural: ineligible: top-level value is not an array | — |
| nyc-opendata | 311-requests | 118.0 | no | structural: ineligible: row 5 has 36 keys, want 35 | — | — | — | no | structural: ineligible: row 0 key "location" holds a nested container | — |
| nyc-opendata | collisions | 70.5 | no | structural: ineligible: row 1 has 14 keys, want 17 | — | — | — | no | structural: ineligible: row 1 has 14 keys, want 17 | — |
| tvmaze | shows | 350.5 | no | structural: ineligible: nested empty array | — | — | — | no | structural: ineligible: row 0 key "_links" holds a nested container | — |
| tvmaze | schedule-us | 296.3 | no | structural: ineligible: nested empty array | — | — | — | no | structural: ineligible: row 0 key "_links" holds a nested container | — |
| pokeapi | pokemon-list | 6.5 | no | structural: ineligible: top-level value is not an array | — | — | — | no | structural: ineligible: top-level value is not an array | — |
| artic | artworks | 454.4 | no | structural: ineligible: top-level value is not an array | — | — | — | no | structural: ineligible: top-level value is not an array | — |
| federalregister | documents | 68.6 | no | structural: ineligible: top-level value is not an array | — | — | — | no | structural: ineligible: top-level value is not an array | — |
| carbonintensity | uk-today | 8.3 | no | structural: ineligible: top-level value is not an array | — | — | — | no | structural: ineligible: top-level value is not an array | — |
| openbrewerydb | breweries | 20.2 | **yes** | — | 6680 | 4387 | 33.0% | **yes** | — | 32.2% |
| worldbank | countries | 37.3 | no | structural: ineligible: row 1 is not an object | — | — | — | no | structural: ineligible: row 1 is not an object | — |

## Inner arrays (top-level objects only)

| API | payload | path | rows | toon | toon gate: reason | JSON tok | TOON tok | toon net | tabular | tabular gate: reason | tabular net |
|---|---|---|---:|---|---|---:|---:|---:|---|---|---:|
| github | go-workflow-runs | `workflow_runs` | 30 | no | structural: ineligible: nested empty array | — | — | — | no | structural: ineligible: row 0 key "actor" holds a nested container | — |
| github | search-repos | `items` | 30 | no | structural: ineligible: number 1.0 does not survive float64 formatting | — | — | — | no | structural: ineligible: row 0 key "license" holds a nested container | — |
| npm | search-react | `objects` | 50 | no | structural: ineligible: column "dependents" mixes kinds | — | — | — | no | structural: ineligible: row 0 key "downloads" holds a nested container | — |
| npm | search-cli | `objects` | 50 | no | net-savings: encoding is not smaller | 15611 | 17579 | -13.2% | no | structural: ineligible: row 0 key "downloads" holds a nested container | — |
| pypi | requests | `releases["2.23.0"]` | 3 | no | structural: ineligible: column "core-metadata" mixes kinds | — | — | — | no | structural: ineligible: row 0 key "digests" holds a nested container | — |
| pypi | flask | `releases["1.1.0"]` | 2 | no | structural: ineligible: column "core-metadata" mixes kinds | — | — | — | no | structural: ineligible: row 0 key "core-metadata" holds a nested cont... | — |
| hn-algolia | search-llm | `hits` | 50 | no | structural: ineligible: row 6 has 14 keys, want 13 | — | — | — | no | structural: ineligible: row 0 key "_highlightResult" holds a nested c... | — |
| hn-algolia | latest-stories | `hits` | 50 | no | structural: ineligible: row 1 has 13 keys, want 12 | — | — | — | no | structural: ineligible: row 0 key "_highlightResult" holds a nested c... | — |
| usgs | quakes-week-m2.5 | `features` | 326 | no | structural: ineligible: toon-go round trip failed | 81299 | — | — | no | structural: ineligible: row 0 key "geometry" holds a nested container | — |
| nws | alerts-ca | `features` | 21 | no | structural: ineligible: nested empty array | — | — | — | no | structural: ineligible: row 0 key "properties" holds a nested container | — |
| stackexchange | so-questions | `items` | 50 | no | structural: ineligible: row 1 has 13 keys, want 14 | — | — | — | no | structural: ineligible: row 0 key "owner" holds a nested container | — |
| openlibrary | search | `docs` | 50 | no | structural: ineligible: row 1 has 14 keys, want 13 | — | — | — | no | structural: ineligible: row 0 key "author_key" holds a nested container | — |
| crossref | works-llm | `message.items` | 30 | no | structural: ineligible: row 1 has 32 keys, want 26 | — | — | — | no | structural: ineligible: row 0 key "ISBN" holds a nested container | — |
| nobelprize | laureates | `laureates` | 50 | no | structural: ineligible: row 1 has 14 keys, want 13 | — | — | — | no | structural: ineligible: row 0 key "birth" holds a nested container | — |
| pokeapi | pokemon-list | `results` | 100 | no | net-savings: net savings 15.0% < 15.0% | 2170 | 1756 | 15.0% | no | net-savings: net savings 11.4% < 15.0% | 11.4% |
| artic | artworks | `data` | 50 | no | structural: ineligible: row 3 has 99 keys, want 98 | — | — | — | no | structural: ineligible: row 0 key "alt_artist_ids" holds a nested con... | — |
| federalregister | documents | `results` | 50 | no | structural: ineligible: toon-go round trip failed | 17042 | — | — | no | structural: ineligible: row 0 key "agencies" holds a nested container | — |
| carbonintensity | uk-today | `data` | 48 | no | net-savings: encoding is not smaller | 2140 | 2526 | -22.2% | no | structural: ineligible: row 0 key "intensity" holds a nested container | — |

Net savings are (JSON − encoded − primer) / JSON against compact canonical JSON. A figure is shown wherever encoding was attempted, including payloads that then failed the net-savings gate.
