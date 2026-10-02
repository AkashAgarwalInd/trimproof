// Package payloads measures, offline, how much realistic tool output
// trimproof's gates accept and what they save. Fetch records responses from
// public, keyless JSON APIs; Analyze runs each one through policy.Decide
// exactly as the gateway would see it as a tool result.
//
// Payloads are third-party content, so they are kept outside the repository
// (DefaultDir); only the source list, checksums and the report are shared.
package payloads

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

// Source is one public JSON endpoint.
type Source struct{ API, Name, URL string }

// Sources are public endpoints that need no key. GitHub's are kept to 12
// because anonymous access allows 60 requests per hour.
var Sources = []Source{
	{"github", "go-issues", "https://api.github.com/repos/golang/go/issues?per_page=50"},
	{"github", "rust-closed-issues", "https://api.github.com/repos/rust-lang/rust/issues?state=closed&per_page=50"},
	{"github", "kubernetes-pulls", "https://api.github.com/repos/kubernetes/kubernetes/pulls?per_page=30"},
	{"github", "cpython-closed-pulls", "https://api.github.com/repos/python/cpython/pulls?state=closed&per_page=30"},
	{"github", "linux-commits", "https://api.github.com/repos/torvalds/linux/commits?per_page=30"},
	{"github", "vscode-releases", "https://api.github.com/repos/microsoft/vscode/releases?per_page=20"},
	{"github", "react-contributors", "https://api.github.com/repos/facebook/react/contributors?per_page=100"},
	{"github", "node-tags", "https://api.github.com/repos/nodejs/node/tags?per_page=100"},
	{"github", "go-labels", "https://api.github.com/repos/golang/go/labels?per_page=100"},
	{"github", "google-repos", "https://api.github.com/orgs/google/repos?per_page=50"},
	{"github", "go-workflow-runs", "https://api.github.com/repos/golang/go/actions/runs?per_page=30"},
	{"github", "search-repos", "https://api.github.com/search/repositories?q=llm+gateway&per_page=30"},
	{"npm", "search-react", "https://registry.npmjs.org/-/v1/search?text=react&size=50"},
	{"npm", "search-cli", "https://registry.npmjs.org/-/v1/search?text=keywords:cli&size=50"},
	{"pypi", "requests", "https://pypi.org/pypi/requests/json"},
	{"pypi", "flask", "https://pypi.org/pypi/flask/json"},
	{"hn", "topstories-ids", "https://hacker-news.firebaseio.com/v0/topstories.json"},
	{"hn", "item", "https://hacker-news.firebaseio.com/v0/item/8863.json"},
	{"hn-algolia", "search-llm", "https://hn.algolia.com/api/v1/search?query=llm&hitsPerPage=50"},
	{"hn-algolia", "latest-stories", "https://hn.algolia.com/api/v1/search_by_date?tags=story&hitsPerPage=50"},
	{"open-meteo", "berlin-hourly", "https://api.open-meteo.com/v1/forecast?latitude=52.52&longitude=13.41&hourly=temperature_2m,relative_humidity_2m,wind_speed_10m&forecast_days=3"},
	{"usgs", "quakes-week-m2.5", "https://earthquake.usgs.gov/earthquakes/feed/v1.0/summary/2.5_week.geojson"},
	{"nws", "alerts-ca", "https://api.weather.gov/alerts/active?area=CA"},
	{"coingecko", "markets", "https://api.coingecko.com/api/v3/coins/markets?vs_currency=usd&per_page=50&page=1"},
	{"coingecko", "exchanges", "https://api.coingecko.com/api/v3/exchanges?per_page=50"},
	{"frankfurter", "rates-september", "https://api.frankfurter.dev/v1/2026-09-01..2026-09-30?to=USD,GBP,JPY"},
	{"stackexchange", "so-questions", "https://api.stackexchange.com/2.3/questions?order=desc&sort=activity&site=stackoverflow&pagesize=50"},
	{"openlibrary", "search", "https://openlibrary.org/search.json?q=distributed+systems&limit=50"},
	{"crossref", "works-llm", "https://api.crossref.org/works?query=large+language+models&rows=30"},
	{"nobelprize", "laureates", "https://api.nobelprize.org/2.1/laureates?limit=50"},
	{"nyc-opendata", "311-requests", "https://data.cityofnewyork.us/resource/erm2-nwe9.json?$limit=100"},
	{"nyc-opendata", "collisions", "https://data.cityofnewyork.us/resource/h9gi-nx95.json?$limit=100"},
	{"tvmaze", "shows", "https://api.tvmaze.com/shows?page=1"},
	{"tvmaze", "schedule-us", "https://api.tvmaze.com/schedule?country=US&date=2026-09-30"},
	{"pokeapi", "pokemon-list", "https://pokeapi.co/api/v2/pokemon?limit=100"},
	{"artic", "artworks", "https://api.artic.edu/api/v1/artworks?limit=50"},
	{"federalregister", "documents", "https://www.federalregister.gov/api/v1/documents.json?per_page=50"},
	{"carbonintensity", "uk-today", "https://api.carbonintensity.org.uk/intensity/date"},
	{"openbrewerydb", "breweries", "https://api.openbrewerydb.org/v1/breweries?per_page=50"},
	{"worldbank", "countries", "https://api.worldbank.org/v2/country?format=json&per_page=100"},
}

// DefaultDir is where fetched payloads are stored, outside the repository.
func DefaultDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(os.TempDir(), "trimproof", "payloads")
	}
	return filepath.Join(home, ".cache", "trimproof", "payloads")
}

// Manifest records what Fetch stored.
type Manifest struct {
	FetchedAt time.Time `json:"fetched_at"`
	Entries   []Entry   `json:"entries"`
}

// Entry is one fetched source. Status is the HTTP status, or 0 with Error
// set when the request failed. File is empty when nothing was stored.
type Entry struct {
	API    string `json:"api"`
	Name   string `json:"name"`
	URL    string `json:"url"`
	File   string `json:"file,omitempty"`
	Bytes  int    `json:"bytes"`
	SHA256 string `json:"sha256,omitempty"`
	Status int    `json:"status"`
	Error  string `json:"error,omitempty"`
}

const (
	userAgent = "trimproof-bench/0.2 (+https://github.com/AkashAgarwalInd/trimproof)"
	maxBytes  = 8 << 20
)

// Fetch GETs every source without credentials, one at a time, and writes
// <api>-<name>.json plus manifest.json to dir. Failed or non-JSON responses
// are recorded in the manifest and skipped.
func Fetch(ctx context.Context, dir string) (Manifest, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return Manifest{}, err
	}
	hc := &http.Client{Timeout: 60 * time.Second}
	m := Manifest{FetchedAt: time.Now().UTC()}
	for i, s := range Sources {
		if i > 0 {
			select {
			case <-ctx.Done():
				return m, ctx.Err()
			case <-time.After(500 * time.Millisecond):
			}
		}
		m.Entries = append(m.Entries, fetchOne(ctx, hc, dir, s))
	}
	b, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return m, err
	}
	return m, os.WriteFile(filepath.Join(dir, "manifest.json"), append(b, '\n'), 0o644)
}

func fetchOne(ctx context.Context, hc *http.Client, dir string, s Source) Entry {
	e := Entry{API: s.API, Name: s.Name, URL: s.URL}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.URL, nil)
	if err != nil {
		e.Error = err.Error()
		return e
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", "application/json")
	resp, err := hc.Do(req)
	if err != nil {
		e.Error = err.Error()
		return e
	}
	defer resp.Body.Close()
	e.Status = resp.StatusCode
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBytes+1))
	switch {
	case err != nil:
		e.Error = err.Error()
	case len(body) > maxBytes:
		e.Error = "response larger than 8 MB"
	case resp.StatusCode != http.StatusOK:
		e.Error = "HTTP " + resp.Status
	case !json.Valid(body):
		e.Error = "response is not JSON"
	}
	if e.Error != "" {
		return e
	}
	sum := sha256.Sum256(body)
	e.File, e.Bytes, e.SHA256 = s.API+"-"+s.Name+".json", len(body), hex.EncodeToString(sum[:])
	if err := os.WriteFile(filepath.Join(dir, e.File), body, 0o644); err != nil {
		e.File, e.Error = "", err.Error()
	}
	return e
}

// ReadManifest loads dir/manifest.json.
func ReadManifest(dir string) (Manifest, error) {
	var m Manifest
	b, err := os.ReadFile(filepath.Join(dir, "manifest.json"))
	if err != nil {
		return m, err
	}
	if err := json.Unmarshal(b, &m); err != nil {
		return m, fmt.Errorf("manifest: %w", err)
	}
	return m, nil
}
