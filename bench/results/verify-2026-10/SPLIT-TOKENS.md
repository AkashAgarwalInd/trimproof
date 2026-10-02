# Offline token counts: wide-table formats, 2026-10-02

Made before any `toonx-split` call, for [Amendment 6](../VERIFY-PLAN.md#amendment-6-split-wide-tables-before-any-result). Counts are o200k_base: the data block plus the format's primer, on the 36 payloads of the payload Q&A set (seed 2004). In brackets: the change against `json-compact`.
- **`toonx-split`:** toonx with every table of more than 8 varying columns cut into tables of at most 8 columns. Each starts with a row-number column `#`.
- **`toonx-narrow`:** toonx when no table is wider than 8 varying columns, otherwise compact JSON. Not sent to models.

Reproduce: `go run ./cmd/bench format-tokens -source payloads -seed 2004 -formats json-compact,toonx2,toonx-split,toonx-narrow`
- The `toonx` column was toonx version 2. Since toonx 3, that format is named `toonx2`.

| dataset | json-compact | toonx | toonx-split | toonx-narrow |
|---|---:|---:|---:|---:|
| payload-github-k8s-issue-comments | 10221 | 5170 (-49.4%) | 5611 (-45.1%) | 10221 (+0.0%) |
| payload-github-torvalds-events | 6801 | 5653 (-16.9%) | 7078 (+4.1%) | 6801 (+0.0%) |
| payload-gitlab-projects | 5865 | 3991 (-32.0%) | 4435 (-24.4%) | 5865 (+0.0%) |
| payload-jsonplaceholder-users | 1234 | 959 (-22.3%) | 1123 (-9.0%) | 1234 (+0.0%) |
| payload-rickandmorty-characters | 5898 | 5009 (-15.1%) | 5181 (-12.2%) | 5898 (+0.0%) |
| payload-wikipedia-recent-changes | 1369 | 1096 (-19.9%) | 1096 (-19.9%) | 1096 (-19.9%) |
| payload-crates-top-downloads | 5624 | 3750 (-33.3%) | 4103 (-27.0%) | 5624 (+0.0%) |
| payload-dockerhub-library-repos | 3896 | 2940 (-24.5%) | 3128 (-19.7%) | 3896 (+0.0%) |
| payload-chicago-data-crimes | 3846 | 2799 (-27.2%) | 3145 (-18.2%) | 3846 (+0.0%) |
| payload-itunes-search-jazz | 13051 | 9946 (-23.8%) | 10397 (-20.3%) | 13051 (+0.0%) |
| payload-citibike-stations | 3534 | 1636 (-53.7%) | 1636 (-53.7%) | 1636 (-53.7%) |
| payload-github-go-issues | 8030 | 6754 (-15.9%) | 7225 (-10.0%) | 8030 (+0.0%) |
| payload-github-rust-closed-issues | 13403 | 8896 (-33.6%) | 9114 (-32.0%) | 13403 (+0.0%) |
| payload-github-linux-commits | 14419 | 11478 (-20.4%) | 11996 (-16.8%) | 14419 (+0.0%) |
| payload-github-vscode-releases | 11666 | 5586 (-52.1%) | 6290 (-46.1%) | 11666 (+0.0%) |
| payload-github-react-contributors | 5105 | 2511 (-50.8%) | 2765 (-45.8%) | 5105 (+0.0%) |
| payload-github-node-tags | 3293 | 2202 (-33.1%) | 2202 (-33.1%) | 2202 (-33.1%) |
| payload-github-go-labels | 1582 | 994 (-37.2%) | 994 (-37.2%) | 994 (-37.2%) |
| payload-github-google-repos | 14268 | 6420 (-55.0%) | 6916 (-51.5%) | 14268 (+0.0%) |
| payload-github-search-repos | 15890 | 9600 (-39.6%) | 10341 (-34.9%) | 15890 (+0.0%) |
| payload-npm-search-react | 6166 | 4490 (-27.2%) | 4838 (-21.5%) | 6166 (+0.0%) |
| payload-npm-search-cli | 5752 | 4252 (-26.1%) | 4600 (-20.0%) | 5752 (+0.0%) |
| payload-hn-algolia-latest-stories | 5317 | 4166 (-21.6%) | 4503 (-15.3%) | 5317 (+0.0%) |
| payload-usgs-quakes-week-m2.5 | 5044 | 2921 (-42.1%) | 3284 (-34.9%) | 5044 (+0.0%) |
| payload-coingecko-markets | 5599 | 3298 (-41.1%) | 3642 (-35.0%) | 5599 (+0.0%) |
| payload-coingecko-exchanges | 4531 | 3686 (-18.6%) | 3887 (-14.2%) | 4531 (+0.0%) |
| payload-stackexchange-so-questions | 4480 | 3526 (-21.3%) | 3889 (-13.2%) | 4480 (+0.0%) |
| payload-openlibrary-search | 2230 | 1622 (-27.3%) | 1902 (-14.7%) | 2230 (+0.0%) |
| payload-crossref-works-llm | 6302 | 5261 (-16.5%) | 5664 (-10.1%) | 6302 (+0.0%) |
| payload-nyc-opendata-311-requests | 6815 | 3691 (-45.8%) | 4124 (-39.5%) | 6815 (+0.0%) |
| payload-nyc-opendata-collisions | 3939 | 1896 (-51.9%) | 2164 (-45.1%) | 3939 (+0.0%) |
| payload-tvmaze-shows | 7589 | 4727 (-37.7%) | 5365 (-29.3%) | 7589 (+0.0%) |
| payload-tvmaze-schedule-us | 11269 | 6414 (-43.1%) | 7136 (-36.7%) | 11269 (+0.0%) |
| payload-pokeapi-pokemon-list | 474 | 318 (-32.9%) | 318 (-32.9%) | 318 (-32.9%) |
| payload-federalregister-documents | 7019 | 5642 (-19.6%) | 5813 (-17.2%) | 7019 (+0.0%) |
| payload-openbrewerydb-breweries | 2707 | 1913 (-29.3%) | 2171 (-19.8%) | 2707 (+0.0%) |
| **all** | **234228** | **155213 (-33.7%)** | **168076 (-28.2%)** | **230222 (-1.7%)** |
