// Package wtq loads WikiTableQuestions (Pasupat & Liang, 2015): real
// Wikipedia tables with crowd-written questions and answers, used as the
// benchmark's non-synthetic accuracy set. The data is CC BY-SA 4.0, so it is
// downloaded on demand into a cache directory and never committed.
package wtq

import (
	"archive/zip"
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"math/rand/v2"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// Release is the official v1.0.2 compact release and its SHA-256.
const (
	ReleaseURL    = "https://github.com/ppasupat/WikiTableQuestions/releases/download/v1.0.2/WikiTableQuestions-1.0.2-compact.zip"
	ReleaseSHA256 = "7c9ca7cc1ccd75fe4be0255b44be63f7b566761005f4ee6ce67e51c129d8b085"
)

// split is the test split whose tables are unseen in training.
const split = "pristine-unseen-tables"

// DefaultDir is the cache directory the release is extracted into.
func DefaultDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(os.TempDir(), "trimproof-wtq")
	}
	return filepath.Join(home, ".cache", "trimproof", "wtq")
}

// Item is one question with its table.
type Item struct {
	ID       string `json:"id"`
	Question string `json:"question"`
	// Answers are the raw target values; Canon their CoreNLP-normalized
	// forms (e.g. "100,000" -> "100000.0"), as the official evaluator uses.
	Answers   []string   `json:"answers"`
	Canon     []string   `json:"canon"`
	Columns   []string   `json:"columns"`
	Cells     [][]string `json:"cells"`
	Rows      int        `json:"rows"`
	TablePath string     `json:"table_path"`
}

// Correct scores a model reply with the official evaluator's rules.
func (it Item) Correct(reply string) bool { return correct(it.Answers, it.Canon, reply) }

// JSON renders the table as a compact JSON array of objects, keys in
// column order and every cell a string, as a tool would return it.
func (it Item) JSON() []byte {
	var b bytes.Buffer
	b.WriteByte('[')
	for i, row := range it.Cells {
		if i > 0 {
			b.WriteByte(',')
		}
		b.WriteByte('{')
		for j, col := range it.Columns {
			if j > 0 {
				b.WriteByte(',')
			}
			quote(&b, col)
			b.WriteByte(':')
			quote(&b, row[j])
		}
		b.WriteByte('}')
	}
	b.WriteByte(']')
	return b.Bytes()
}

// quote writes s as a JSON string without HTML escaping.
func quote(b *bytes.Buffer, s string) {
	enc := json.NewEncoder(b)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(s)
	b.Truncate(b.Len() - 1) // Encode appends a newline
}

// Fetch downloads the release, verifies its checksum and extracts the
// question files and TSV tables into dir. It does nothing when dir already
// holds a verified extraction.
func Fetch(ctx context.Context, dir string) error {
	marker := filepath.Join(dir, ".sha256")
	if b, err := os.ReadFile(marker); err == nil && strings.TrimSpace(string(b)) == ReleaseSHA256 {
		return nil
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, ReleaseURL, nil)
	if err != nil {
		return err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("wtq: download: %s", resp.Status)
	}
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	sum := sha256.Sum256(data)
	if got := hex.EncodeToString(sum[:]); got != ReleaseSHA256 {
		return fmt.Errorf("wtq: checksum %s, want %s", got, ReleaseSHA256)
	}
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return err
	}
	for _, f := range zr.File {
		name, ok := strings.CutPrefix(f.Name, "WikiTableQuestions/")
		if !ok || !wanted(name) {
			continue
		}
		if err := extract(f, dir, name); err != nil {
			return err
		}
	}
	return os.WriteFile(marker, []byte(ReleaseSHA256+"\n"), 0o644)
}

// wanted keeps the tagged questions, the TSV tables and the license notes.
func wanted(name string) bool {
	switch {
	case strings.HasPrefix(name, "tagged/data/") && strings.HasSuffix(name, ".tagged"),
		strings.HasPrefix(name, "csv/") && strings.HasSuffix(name, ".tsv"),
		name == "README.md", name == "evaluator.py":
		return true
	}
	return false
}

func extract(f *zip.File, dir, name string) error {
	dst := filepath.Join(dir, filepath.FromSlash(name))
	if !strings.HasPrefix(dst, filepath.Clean(dir)+string(filepath.Separator)) {
		return fmt.Errorf("wtq: bad path %q in archive", f.Name)
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	rc, err := f.Open()
	if err != nil {
		return err
	}
	defer rc.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, rc); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}

// Load reads the pristine-unseen-tables test split from dir, keeps
// questions whose table has at least minRows rows, and returns a seeded
// random sample of n of them (all of them when n <= 0), ordered by ID.
func Load(dir string, n, minRows int, seed uint64) ([]Item, error) {
	f, err := os.Open(filepath.Join(dir, "tagged", "data", split+".tagged"))
	if err != nil {
		return nil, err
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 1<<24)
	if !sc.Scan() {
		return nil, fmt.Errorf("wtq: empty question file")
	}
	col := map[string]int{}
	for i, h := range strings.Split(sc.Text(), "\t") {
		col[h] = i
	}
	for _, h := range []string{"id", "utterance", "context", "targetValue", "targetCanon"} {
		if _, ok := col[h]; !ok {
			return nil, fmt.Errorf("wtq: question file has no %q column", h)
		}
	}
	tables := map[string]*table{}
	var items []Item
	for sc.Scan() {
		fs := strings.Split(sc.Text(), "\t")
		if len(fs) < len(col) {
			continue
		}
		path := strings.TrimSuffix(fs[col["context"]], ".csv") + ".tsv"
		t, ok := tables[path]
		if !ok {
			if t, err = readTable(filepath.Join(dir, filepath.FromSlash(path))); err != nil {
				return nil, err
			}
			tables[path] = t
		}
		if len(t.cells) < minRows {
			continue
		}
		items = append(items, Item{
			ID:        fs[col["id"]],
			Question:  fs[col["utterance"]],
			Answers:   unescapeList(fs[col["targetValue"]]),
			Canon:     unescapeList(fs[col["targetCanon"]]),
			Columns:   t.columns,
			Cells:     t.cells,
			Rows:      len(t.cells),
			TablePath: path,
		})
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	slices.SortFunc(items, func(a, b Item) int { return strings.Compare(a.ID, b.ID) })
	if n > 0 && n < len(items) {
		r := rand.New(rand.NewPCG(seed, seed^0x5eed))
		pick := r.Perm(len(items))[:n]
		slices.Sort(pick)
		sample := make([]Item, n)
		for i, j := range pick {
			sample[i] = items[j]
		}
		items = sample
	}
	return items, nil
}

type table struct {
	columns []string
	cells   [][]string
}

// readTable parses a WTQ table TSV: the first line is the header, and
// newlines, pipes and backslashes are escaped as \n, \p and \\.
func readTable(path string) (*table, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	lines := strings.Split(strings.TrimRight(string(b), "\n"), "\n")
	t := &table{columns: columnNames(strings.Split(lines[0], "\t"))}
	for _, l := range lines[1:] {
		fs := strings.Split(l, "\t")
		row := make([]string, len(t.columns))
		for i := range row {
			if i < len(fs) {
				row[i] = unescape(fs[i])
			}
		}
		t.cells = append(t.cells, row)
	}
	return t, nil
}

// columnNames makes header cells usable as JSON keys: newlines become
// spaces, empty names become col<i>, and repeats get _2, _3, ...
func columnNames(raw []string) []string {
	names := make([]string, len(raw))
	used := map[string]bool{}
	for i, h := range raw {
		h = strings.Join(strings.Fields(unescape(h)), " ")
		if h == "" {
			h = fmt.Sprintf("col%d", i)
		}
		for k, base := 2, h; used[h]; k++ {
			h = fmt.Sprintf("%s_%d", base, k)
		}
		used[h] = true
		names[i] = h
	}
	return names
}

func unescape(s string) string {
	if !strings.Contains(s, `\`) {
		return s
	}
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] == '\\' && i+1 < len(s) {
			switch s[i+1] {
			case 'n':
				b.WriteByte('\n')
				i++
				continue
			case 'p':
				b.WriteByte('|')
				i++
				continue
			case '\\':
				b.WriteByte('\\')
				i++
				continue
			}
		}
		b.WriteByte(s[i])
	}
	return b.String()
}

func unescapeList(s string) []string {
	parts := strings.Split(s, "|")
	for i, p := range parts {
		parts[i] = unescape(p)
	}
	return parts
}
