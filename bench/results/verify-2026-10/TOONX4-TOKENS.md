# Offline token counts: toonx 4, 2026-10-02

Made before any toonx 4 call, for [Amendment 7](../VERIFY-PLAN.md#amendment-7-row-key-and-marked-prefixes-before-any-result). Counts are o200k_base, data block plus primer, on the 36 payloads of the payload Q&A set (seed 2004). In brackets: the change against `json-compact`.
- `toonx2`: version 2, no split.
- `toonx3`: version 3, as tested in pilot 3.
- `toonx`: version 4. Each part starts with the row's key field, and prefixed values start with `~`.

Reproduce: `go run ./cmd/bench format-tokens -source payloads -seed 2004 -formats json-compact,toonx2,toonx3,toonx`

| dataset | json-compact | toonx2 | toonx3 | toonx |
|---|---:|---:|---:|---:|
| payload-github-k8s-issue-comments | 10221 | 5170 (-49.4%) | 5611 (-45.1%) | 6119 (-40.1%) |
| payload-github-torvalds-events | 6801 | 5653 (-16.9%) | 7078 (+4.1%) | 8251 (+21.3%) |
| payload-gitlab-projects | 5865 | 3991 (-32.0%) | 4435 (-24.4%) | 4664 (-20.5%) |
| payload-jsonplaceholder-users | 1234 | 959 (-22.3%) | 1123 (-9.0%) | 1128 (-8.6%) |
| payload-rickandmorty-characters | 5898 | 5009 (-15.1%) | 5181 (-12.2%) | 5202 (-11.8%) |
| payload-wikipedia-recent-changes | 1369 | 1096 (-19.9%) | 1096 (-19.9%) | 1096 (-19.9%) |
| payload-crates-top-downloads | 5624 | 3750 (-33.3%) | 4103 (-27.0%) | 4018 (-28.6%) |
| payload-dockerhub-library-repos | 3896 | 2940 (-24.5%) | 3128 (-19.7%) | 3085 (-20.8%) |
| payload-chicago-data-crimes | 3846 | 2799 (-27.2%) | 3145 (-18.2%) | 3275 (-14.8%) |
| payload-itunes-search-jazz | 13051 | 9946 (-23.8%) | 10397 (-20.3%) | 10711 (-17.9%) |
| payload-citibike-stations | 3534 | 1636 (-53.7%) | 1636 (-53.7%) | 1636 (-53.7%) |
| payload-github-go-issues | 8030 | 6754 (-15.9%) | 7225 (-10.0%) | 7628 (-5.0%) |
| payload-github-rust-closed-issues | 13403 | 8896 (-33.6%) | 9114 (-32.0%) | 9450 (-29.5%) |
| payload-github-linux-commits | 14419 | 11478 (-20.4%) | 11996 (-16.8%) | 15710 (+9.0%) |
| payload-github-vscode-releases | 11666 | 5586 (-52.1%) | 6290 (-46.1%) | 6909 (-40.8%) |
| payload-github-react-contributors | 5105 | 2511 (-50.8%) | 2765 (-45.8%) | 3093 (-39.4%) |
| payload-github-node-tags | 3293 | 2202 (-33.1%) | 2202 (-33.1%) | 2289 (-30.5%) |
| payload-github-go-labels | 1582 | 994 (-37.2%) | 994 (-37.2%) | 1012 (-36.0%) |
| payload-github-google-repos | 14268 | 6420 (-55.0%) | 6916 (-51.5%) | 7537 (-47.2%) |
| payload-github-search-repos | 15890 | 9600 (-39.6%) | 10341 (-34.9%) | 11217 (-29.4%) |
| payload-npm-search-react | 6166 | 4490 (-27.2%) | 4838 (-21.5%) | 4876 (-20.9%) |
| payload-npm-search-cli | 5752 | 4252 (-26.1%) | 4600 (-20.0%) | 4648 (-19.2%) |
| payload-hn-algolia-latest-stories | 5317 | 4166 (-21.6%) | 4503 (-15.3%) | 4623 (-13.1%) |
| payload-usgs-quakes-week-m2.5 | 5044 | 2921 (-42.1%) | 3284 (-34.9%) | 3484 (-30.9%) |
| payload-coingecko-markets | 5599 | 3298 (-41.1%) | 3642 (-35.0%) | 3642 (-35.0%) |
| payload-coingecko-exchanges | 4531 | 3686 (-18.6%) | 3887 (-14.2%) | 3873 (-14.5%) |
| payload-stackexchange-so-questions | 4480 | 3526 (-21.3%) | 3889 (-13.2%) | 4127 (-7.9%) |
| payload-openlibrary-search | 2230 | 1622 (-27.3%) | 1902 (-14.7%) | 1897 (-14.9%) |
| payload-crossref-works-llm | 6302 | 5261 (-16.5%) | 5664 (-10.1%) | 5677 (-9.9%) |
| payload-nyc-opendata-311-requests | 6815 | 3691 (-45.8%) | 4124 (-39.5%) | 4280 (-37.2%) |
| payload-nyc-opendata-collisions | 3939 | 1896 (-51.9%) | 2164 (-45.1%) | 2240 (-43.1%) |
| payload-tvmaze-shows | 7589 | 4727 (-37.7%) | 5365 (-29.3%) | 5460 (-28.1%) |
| payload-tvmaze-schedule-us | 11269 | 6414 (-43.1%) | 7136 (-36.7%) | 7486 (-33.6%) |
| payload-pokeapi-pokemon-list | 474 | 318 (-32.9%) | 318 (-32.9%) | 336 (-29.1%) |
| payload-federalregister-documents | 7019 | 5642 (-19.6%) | 5813 (-17.2%) | 5871 (-16.4%) |
| payload-openbrewerydb-breweries | 2707 | 1913 (-29.3%) | 2171 (-19.8%) | 2503 (-7.5%) |
| **all** | **234228** | **155213 (-33.7%)** | **168076 (-28.2%)** | **179053 (-23.6%)** |
