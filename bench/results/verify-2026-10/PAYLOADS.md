# Real API payloads through trimproof's gates

Fetched 2026-10-02 12:51 UTC from 40 public, keyless endpoints; 40 stored.

Each response body is judged by `policy.Decide` exactly as sent, as one tool result, under the default route policy with the codec ENABLED: strict schema, minimum 200 tokens, minimum net savings 15% with the primer counted. Token counts are o200k. Net savings are (JSON − encoded − primer) / JSON against compact canonical JSON.

## Overall

| | toonx | toon | tabular |
|---|---:|---:|---:|
| payloads eligible | 27 / 40 (68%) | 3 / 40 (8%) | 3 / 40 (8%) |
| median net savings, eligible payloads | 38.7% | 28.9% | 27.7% |
| token-weighted net savings, eligible payloads | 41.0% | 25.5% | 25.3% |
| token-weighted net savings, all payloads (ineligible ones save 0) | 30.2% | 0.6% | 0.6% |
| median savings vs pretty-printed JSON, eligible payloads | 50.9% | 47.1% | 46.2% |

## By API

| API | payloads | toonx eligible | toon eligible | tabular eligible |
|---|---:|---:|---:|---:|
| github | 12 | 12 | 2 | 2 |
| npm | 2 | 2 | 0 | 0 |
| pypi | 2 | 0 | 0 | 0 |
| hn | 2 | 0 | 0 | 0 |
| hn-algolia | 2 | 1 | 0 | 0 |
| open-meteo | 1 | 0 | 0 | 0 |
| usgs | 1 | 1 | 0 | 0 |
| nws | 1 | 0 | 0 | 0 |
| coingecko | 2 | 2 | 0 | 0 |
| frankfurter | 1 | 0 | 0 | 0 |
| stackexchange | 1 | 0 | 0 | 0 |
| openlibrary | 1 | 1 | 0 | 0 |
| crossref | 1 | 0 | 0 | 0 |
| nobelprize | 1 | 0 | 0 | 0 |
| nyc-opendata | 2 | 2 | 0 | 0 |
| tvmaze | 2 | 2 | 0 | 0 |
| pokeapi | 1 | 1 | 0 | 0 |
| artic | 1 | 0 | 0 | 0 |
| federalregister | 1 | 1 | 0 | 0 |
| carbonintensity | 1 | 1 | 0 | 0 |
| openbrewerydb | 1 | 1 | 1 | 1 |
| worldbank | 1 | 0 | 0 | 0 |

## Per payload

Each codec cell is the net saving when the payload is eligible (**bold**), else the net saving where encoding was attempted, then the rejecting gate.

| API | payload | KB | JSON tok | toonx | toon | tabular |
|---|---|---:|---:|---|---|---|
| github | go-issues | 242.2 | 73213 | **28.3%** | structural: ineligible: toon-go round trip failed | structural: ineligible: row 0 key "assignee" holds a nested container |
| github | rust-closed-issues | 375.8 | 112853 | **31.7%** | structural: ineligible: toon-go round trip failed | structural: ineligible: row 0 key "assignees" holds a nested container |
| github | kubernetes-pulls | 703.7 | 194825 | **43.5%** | structural: ineligible: toon-go round trip failed | structural: ineligible: row 0 key "_links" holds a nested container |
| github | cpython-closed-pulls | 533.5 | 150329 | **54.4%** | structural: ineligible: toon-go round trip failed | structural: ineligible: row 0 key "_links" holds a nested container |
| github | linux-commits | 123.6 | 41013 | **22.1%** | structural: ineligible: toon-go round trip failed | structural: ineligible: row 0 key "author" holds a nested container |
| github | vscode-releases | 38.5 | 11666 | **46.1%** | -13.7% · net-savings: encoding is not smaller | structural: ineligible: row 0 key "assets" holds a nested container |
| github | react-contributors | 93.3 | 25809 | **50.3%** | **22.5%** | **22.8%** |
| github | node-tags | 39.2 | 16306 | **39.0%** | -6.7% · net-savings: encoding is not smaller | structural: ineligible: row 0 key "commit" holds a nested container |
| github | go-labels | 25.9 | 8048 | **45.5%** | **28.9%** | **27.7%** |
| github | google-repos | 268.6 | 70959 | **58.6%** | -10.2% · net-savings: encoding is not smaller | structural: ineligible: row 0 key "custom_properties" holds a nested ... |
| github | go-workflow-runs | 355.9 | 99976 | **56.6%** | structural: ineligible: toon-go round trip failed | structural: ineligible: top-level value is not an array |
| github | search-repos | 161.3 | 46750 | **40.7%** | structural: ineligible: number 1.0 does not survive float64 formatting | structural: ineligible: top-level value is not an array |
| npm | search-react | 52.0 | 15339 | **25.5%** | -14.2% · net-savings: encoding is not smaller | structural: ineligible: top-level value is not an array |
| npm | search-cli | 52.1 | 15636 | **22.2%** | -13.2% · net-savings: encoding is not smaller | structural: ineligible: top-level value is not an array |
| pypi | requests | 188.5 | 82258 | 2.1% · net-savings: net savings 2.1% < 15.0% | -8.8% · net-savings: encoding is not smaller | structural: ineligible: top-level value is not an array |
| pypi | flask | 86.7 | 38323 | 2.0% · net-savings: net savings 2.0% < 15.0% | -8.4% · net-savings: encoding is not smaller | structural: ineligible: top-level value is not an array |
| hn | topstories-ids | 4.4 | 2001 | -4.5% · net-savings: encoding is not smaller | -4.5% · net-savings: encoding is not smaller | structural: ineligible: row 0 is not an object |
| hn | item | 0.4 | 165 | min-size: 165 < 200 tokens | min-size: 165 < 200 tokens | structural: ineligible: top-level value is not an array |
| hn-algolia | search-llm | 75.6 | 27887 | 11.2% · net-savings: net savings 11.2% < 15.0% | -7.7% · net-savings: encoding is not smaller | structural: ineligible: top-level value is not an array |
| hn-algolia | latest-stories | 40.9 | 12142 | **21.0%** | -15.4% · net-savings: encoding is not smaller | structural: ineligible: top-level value is not an array |
| open-meteo | berlin-hourly | 2.5 | 1624 | -6.7% · net-savings: encoding is not smaller | structural: ineligible: number 38.0 does not survive float64 formatting | structural: ineligible: top-level value is not an array |
| usgs | quakes-week-m2.5 | 225.7 | 81410 | **39.7%** | -17.2% · net-savings: encoding is not smaller | structural: ineligible: top-level value is not an array |
| nws | alerts-ca | 103.0 | 22112 | 14.1% · net-savings: net savings 14.1% < 15.0% | -7.0% · net-savings: encoding is not smaller | structural: ineligible: top-level value is not an array |
| coingecko | markets | 38.2 | 14037 | **38.7%** | structural: ineligible: number 20092406.0 does not survive float64 fo... | structural: ineligible: row 1 key "roi" holds a nested container |
| coingecko | exchanges | 51.4 | 12041 | **16.5%** | 14.9% · net-savings: net savings 14.9% < 15.0% | 14.8% · net-savings: net savings 14.8% < 15.0% |
| frankfurter | rates-september | 1.2 | 625 | -45.9% · net-savings: encoding is not smaller | structural: ineligible: number 1.0 does not survive float64 formatting | structural: ineligible: top-level value is not an array |
| stackexchange | so-questions | 37.4 | 11119 | 8.3% · net-savings: net savings 8.3% < 15.0% | -15.9% · net-savings: encoding is not smaller | structural: ineligible: top-level value is not an array |
| openlibrary | search | 19.0 | 5480 | **22.7%** | -18.9% · net-savings: encoding is not smaller | structural: ineligible: top-level value is not an array |
| crossref | works-llm | 178.6 | 58271 | 4.2% · net-savings: net savings 4.2% < 15.0% | structural: ineligible: toon-go round trip failed | structural: ineligible: top-level value is not an array |
| nobelprize | laureates | 186.7 | 56472 | 13.5% · net-savings: net savings 13.5% < 15.0% | structural: ineligible: toon-go round trip failed | structural: ineligible: top-level value is not an array |
| nyc-opendata | 311-requests | 118.0 | 35285 | **40.0%** | -13.5% · net-savings: encoding is not smaller | structural: ineligible: row 0 key "location" holds a nested container |
| nyc-opendata | collisions | 70.5 | 21561 | **44.3%** | -14.1% · net-savings: encoding is not smaller | structural: ineligible: row 1 has 14 keys, want 17 |
| tvmaze | shows | 350.5 | 96752 | **32.6%** | -13.5% · net-savings: encoding is not smaller | structural: ineligible: row 0 key "_links" holds a nested container |
| tvmaze | schedule-us | 296.3 | 85712 | **37.5%** | -13.6% · net-savings: encoding is not smaller | structural: ineligible: row 0 key "_links" holds a nested container |
| pokeapi | pokemon-list | 6.5 | 2202 | **62.2%** | 14.7% · net-savings: net savings 14.7% < 15.0% | structural: ineligible: top-level value is not an array |
| artic | artworks | 454.4 | 141499 | 12.8% · net-savings: net savings 12.8% < 15.0% | structural: ineligible: number 2.666754776244474e-6 does not survive ... | structural: ineligible: top-level value is not an array |
| federalregister | documents | 68.6 | 17115 | **20.4%** | structural: ineligible: toon-go round trip failed | structural: ineligible: top-level value is not an array |
| carbonintensity | uk-today | 8.3 | 2142 | **21.5%** | -22.1% · net-savings: encoding is not smaller | structural: ineligible: top-level value is not an array |
| openbrewerydb | breweries | 20.2 | 6680 | **24.9%** | **33.0%** | **32.2%** |
| worldbank | countries | 37.3 | 11264 | -12.2% · net-savings: encoding is not smaller | -20.1% · net-savings: encoding is not smaller | structural: ineligible: row 1 is not an object |
