package bench

import (
	"cmp"
	"encoding/json"
	"fmt"
	"math/big"
	"math/rand/v2"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/AkashAgarwalInd/trimproof/internal/bench/payloads"
	"github.com/AkashAgarwalInd/trimproof/internal/bench/wtq"
	"github.com/AkashAgarwalInd/trimproof/pkg/canonical"
	"github.com/AkashAgarwalInd/trimproof/pkg/tokens"
)

// Payload Q&A limits: the main table is cut to its first payloadRows rows,
// halved until the payload is at most payloadMaxTokens (o200k, compact
// JSON), and never below payloadMinRows rows.
const (
	payloadRows         = 20
	payloadMinRows      = 5
	payloadMaxTokens    = 16000
	payloadMaxQuestions = 4 // per payload
)

// PayloadItem records one payload of the Q&A set, for its manifest.
type PayloadItem struct {
	Set       string     `json:"set"` // tuning | held-out
	API       string     `json:"api"`
	Name      string     `json:"name"`
	URL       string     `json:"url"`
	SHA256    string     `json:"sha256"` // of the stored response body
	Path      string     `json:"table_path"`
	TotalRows int        `json:"table_rows"`
	KeptRows  int        `json:"rows_kept"`
	JSONBytes int        `json:"json_bytes"`
	Questions []Question `json:"-"`
}

func payloadKey(set, api, name string) string { return set + "/" + api + "/" + name }

// ReadPayloadSet reads a payload Q&A manifest (as WritePayloadManifest
// writes it) and returns its codec label and its payloads' checksums by
// set/api/name.
func ReadPayloadSet(path string) (string, map[string]string, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return "", nil, err
	}
	var m struct {
		Codec string        `json:"codec"`
		Items []PayloadItem `json:"items"`
	}
	if err := json.Unmarshal(b, &m); err != nil {
		return "", nil, fmt.Errorf("%s: %w", path, err)
	}
	sums := map[string]string{}
	for _, it := range m.Items {
		sums[payloadKey(it.Set, it.API, it.Name)] = it.SHA256
	}
	return m.Codec, sums, nil
}

// PayloadDatasets builds questions over real API responses (one dataset per
// payload, from each directory's manifest in order). Each payload's largest
// array of objects is its table; it is cut to fit the limits above. With
// registered (from ReadPayloadSet), exactly the registered payloads are
// kept, and each must have its registered checksum; with nil, a payload is
// kept if the gateway's default gates would encode it with toonx. Questions
// and gold answers are computed from the data; replies are scored with the
// WikiTableQuestions evaluator. The JSON arms send the cut payload as
// canonical compact JSON.
func PayloadDatasets(sets map[string]string, registered map[string]string, seed uint64) ([]*Dataset, []PayloadItem, error) {
	var ds []*Dataset
	var items []PayloadItem
	est := tokens.NewCalibrated(nil)
	stream := uint64(0)
	for _, set := range slices.Sorted(func(yield func(string) bool) {
		for k := range sets {
			if !yield(k) {
				return
			}
		}
	}) {
		dir := sets[set]
		m, err := payloads.ReadManifest(dir)
		if err != nil {
			return nil, nil, err
		}
		for _, e := range m.Entries {
			if e.File == "" {
				continue
			}
			stream++
			raw, err := os.ReadFile(filepath.Join(dir, e.File))
			if err != nil {
				return nil, nil, err
			}
			key := payloadKey(set, e.API, e.Name)
			sum, isRegistered := registered[key]
			if registered != nil && !isRegistered {
				continue
			}
			if isRegistered && sum != e.SHA256 {
				return nil, nil, fmt.Errorf("payload %s: stored body has sha256 %s, registered %s", key, e.SHA256, sum)
			}
			it, body, ok := payloadItem(raw, est)
			if !ok {
				continue
			}
			if registered == nil {
				o, err := payloads.Evaluate(body, "toonx", est)
				if err != nil {
					return nil, nil, err
				}
				if !o.Eligible {
					continue
				}
			}
			it.Set, it.API, it.Name, it.URL, it.SHA256 = set, e.API, e.Name, e.URL, e.SHA256
			r := rand.New(rand.NewPCG(seed, stream))
			it.Questions = payloadQuestions(r, it.Path, tableOf(body, it.Path))
			if len(it.Questions) == 0 {
				continue
			}
			id := fmt.Sprintf("payload-%s-%s", e.API, e.Name)
			for j := range it.Questions {
				it.Questions[j].ID = fmt.Sprintf("%s-q%d", id, j+1)
			}
			items = append(items, it)
			qs := make([]Question, len(it.Questions))
			for j, q := range it.Questions {
				answers := q.Answers
				q.Check = func(reply string) bool { return wtq.Correct(answers, reply) }
				qs[j] = q
			}
			ds = append(ds, &Dataset{ID: id, Name: "payload", Rows: it.KeptRows, Tool: "http_get",
				ToolArgs: fmt.Sprintf(`{"url":%q}`, e.URL), Raw: body, Questions: qs})
		}
	}
	if registered != nil && len(items) != len(registered) {
		return nil, nil, fmt.Errorf("found %d of the %d registered payloads", len(items), len(registered))
	}
	return ds, items, nil
}

// payloadItem finds raw's table, cuts it to the limits and returns the cut
// payload as canonical JSON. ok is false when there is no table of at least
// payloadMinRows rows or the payload cannot fit.
func payloadItem(raw []byte, est tokens.Estimator) (PayloadItem, []byte, bool) {
	v, err := canonical.Parse(raw)
	if err != nil {
		return PayloadItem{}, nil, false
	}
	path, total := largestTable(v)
	if total < payloadMinRows {
		return PayloadItem{}, nil, false
	}
	for keep := min(total, payloadRows); keep >= payloadMinRows; keep /= 2 {
		cut := withRows(v, path, keep)
		b, err := canonical.Marshal(cut)
		if err != nil {
			return PayloadItem{}, nil, false
		}
		if est.Estimate(string(b), payloads.Model) <= payloadMaxTokens {
			return PayloadItem{Path: strings.Join(path, "."), TotalRows: total, KeptRows: keep, JSONBytes: len(b)}, b, true
		}
	}
	return PayloadItem{}, nil, false
}

// largestTable returns the path of the array of objects with the most rows
// (ties: the first in key order), searching objects up to three levels down.
func largestTable(v any) ([]string, int) {
	var best []string
	n := 0
	var walk func(v any, path []string)
	walk = func(v any, path []string) {
		switch t := v.(type) {
		case []any:
			if len(t) > n && allObjects(t) {
				best, n = slices.Clone(path), len(t)
			}
		case map[string]any:
			if len(path) == 3 {
				return
			}
			for _, k := range canonical.SortedKeys(t) {
				if strings.Contains(k, ".") {
					continue // keeps "a.b" unambiguous as a path
				}
				walk(t[k], append(path, k))
			}
		}
	}
	walk(v, nil)
	return best, n
}

func allObjects(a []any) bool {
	for _, x := range a {
		if _, ok := x.(map[string]any); !ok {
			return false
		}
	}
	return len(a) > 0
}

// withRows returns v with the array at path cut to its first n elements.
func withRows(v any, path []string, n int) any {
	if len(path) == 0 {
		return v.([]any)[:n]
	}
	m := v.(map[string]any)
	out := make(map[string]any, len(m))
	for k, x := range m {
		out[k] = x
	}
	out[path[0]] = withRows(m[path[0]], path[1:], n)
	return out
}

func tableOf(body []byte, path string) []map[string]any {
	v, _ := canonical.Parse(body)
	if path != "" {
		for _, k := range strings.Split(path, ".") {
			v = v.(map[string]any)[k]
		}
	}
	var rows []map[string]any
	for _, x := range v.([]any) {
		rows = append(rows, x.(map[string]any))
	}
	return rows
}

// leaves flattens a row into dotted paths to its primitive values; arrays
// and empty objects are left out.
func leaves(prefix string, m map[string]any, out map[string]any) {
	for k, v := range m {
		if strings.Contains(k, ".") {
			continue
		}
		switch t := v.(type) {
		case map[string]any:
			leaves(prefix+k+".", t, out)
		case []any:
		default:
			out[prefix+k] = v
		}
	}
}

func text(v any) string {
	switch t := v.(type) {
	case nil:
		return "null"
	case string:
		return t
	default:
		return fmt.Sprint(t)
	}
}

// usable reports whether v makes a clear answer: a short single-line
// string, a number or a boolean.
func usable(v any) bool {
	switch t := v.(type) {
	case string:
		return t != "" && len(t) <= 60 && !strings.ContainsAny(t, "\n|") && strings.TrimSpace(t) == t
	case json.Number, bool:
		return true
	}
	return false
}

// payloadQuestions writes up to payloadMaxQuestions questions about rows: a
// lookup of a nested field when there is one, a lookup of a field some rows
// lack, a count of rows with a given value, and the row with the largest
// number.
func payloadQuestions(r *rand.Rand, path string, rows []map[string]any) []Question {
	where := "the result"
	if path != "" {
		where = fmt.Sprintf("the %q list", path)
	}
	flat := make([]map[string]any, len(rows))
	cols := map[string]bool{}
	for i, row := range rows {
		flat[i] = map[string]any{}
		leaves("", row, flat[i])
		for c := range flat[i] {
			cols[c] = true
		}
	}
	names := slices.Sorted(func(yield func(string) bool) {
		for c := range cols {
			if !yield(c) {
				return
			}
		}
	})
	id := idColumn(names, flat)
	if id == "" {
		return nil
	}
	present := func(c string) int {
		n := 0
		for _, f := range flat {
			if v, ok := f[c]; ok && v != nil {
				n++
			}
		}
		return n
	}
	var qs []Question
	lookup := func(kind, c string, row int) {
		v, ok := flat[row][c]
		ans := "none"
		if ok && v != nil {
			if !usable(v) {
				return
			}
			ans = text(v)
		}
		qs = append(qs, Question{Kind: kind, Answer: ans, Answers: []string{ans},
			Text: fmt.Sprintf("In %s, what is %q for the item whose %q is %s? If that item has no %q value, answer none.",
				where, c, id, text(flat[row][id]), c)})
	}
	pick := func(xs []string) string { return xs[r.IntN(len(xs))] }

	// 1. A nested field, else any field, of a random row.
	var nested, plain, sparse []string
	for _, c := range names {
		if c == id {
			continue
		}
		p := present(c)
		switch {
		case p == 0: // null or absent everywhere: nothing to ask
		case p < len(flat):
			sparse = append(sparse, c)
		case strings.Contains(c, "."):
			nested = append(nested, c)
		default:
			plain = append(plain, c)
		}
	}
	r.Shuffle(len(nested), func(i, j int) { nested[i], nested[j] = nested[j], nested[i] })
	r.Shuffle(len(plain), func(i, j int) { plain[i], plain[j] = plain[j], plain[i] })
	more := append(nested, plain...) // further lookups, if questions run short
	if len(more) > 0 {
		kind := "lookup"
		if len(nested) > 0 {
			kind = "lookup-nested"
		}
		lookup(kind, more[0], r.IntN(len(flat)))
		more = more[1:]
	}
	// 2. A field that some rows lack or hold null: half the time on a row
	// without it.
	if len(sparse) > 0 {
		c := pick(sparse)
		var with, without []int
		for i, f := range flat {
			if v, ok := f[c]; ok && v != nil {
				with = append(with, i)
			} else {
				without = append(without, i)
			}
		}
		rows := with
		if r.IntN(2) == 0 {
			rows = without
		}
		lookup("lookup-sparse", c, rows[r.IntN(len(rows))])
	}
	// 3. Count of rows with a value, over a column of 2–6 distinct values.
	var cats []string
	for _, c := range names {
		vals := map[string]bool{}
		ok := true
		for _, f := range flat {
			v, has := f[c]
			if !has || v == nil {
				continue
			}
			if !usable(v) {
				ok = false
				break
			}
			vals[text(v)] = true
		}
		if ok && c != id && len(vals) >= 2 && len(vals) <= 6 {
			cats = append(cats, c)
		}
	}
	if len(cats) > 0 {
		c := pick(cats)
		v := text(flat[r.IntN(len(flat))][c])
		for i := 0; v == "null" && i < len(flat); i++ {
			v = text(flat[i][c])
		}
		n := 0
		for _, f := range flat {
			if x, ok := f[c]; ok && x != nil && text(x) == v {
				n++
			}
		}
		qs = append(qs, Question{Kind: "count-where", Answer: fmt.Sprint(n), Answers: []string{fmt.Sprint(n)}, Numeric: true,
			Text: fmt.Sprintf("How many items in %s have %q equal to %s?", where, c, v)})
	}
	// 4. The row with the largest value of a numeric column that every row
	// has, when the largest is unique.
	var nums []string
	for _, c := range names {
		if c == id || present(c) != len(flat) {
			continue
		}
		all := true
		for _, f := range flat {
			if _, ok := f[c].(json.Number); !ok {
				all = false
				break
			}
		}
		if all {
			nums = append(nums, c)
		}
	}
	r.Shuffle(len(nums), func(i, j int) { nums[i], nums[j] = nums[j], nums[i] })
	for _, c := range nums {
		vals := make([]*big.Rat, len(flat))
		for i, f := range flat {
			vals[i], _ = new(big.Rat).SetString(string(f[c].(json.Number)))
		}
		top := slices.MaxFunc(slices.Collect(func(yield func(int) bool) {
			for i := range vals {
				if !yield(i) {
					return
				}
			}
		}), func(a, b int) int { return vals[a].Cmp(vals[b]) })
		ties := 0
		for _, v := range vals {
			if v.Cmp(vals[top]) == 0 {
				ties++
			}
		}
		if ties == 1 {
			ans := text(flat[top][id])
			qs = append(qs, Question{Kind: "argmax", Answer: ans, Answers: []string{ans},
				Text: fmt.Sprintf("Which item in %s has the largest %q? Answer with its %q.", where, c, id)})
			break
		}
	}
	for len(qs) < payloadMaxQuestions && len(more) > 0 {
		kind := "lookup"
		if strings.Contains(more[0], ".") {
			kind = "lookup-nested"
		}
		lookup(kind, more[0], r.IntN(len(flat)))
		more = more[1:]
	}
	if len(qs) > payloadMaxQuestions {
		qs = qs[:payloadMaxQuestions]
	}
	return qs
}

// idColumn picks the column that names rows: present and usable in every
// row, unique, preferring familiar names.
func idColumn(names []string, flat []map[string]any) string {
	var ids []string
	for _, c := range names {
		seen := map[string]bool{}
		ok := true
		for _, f := range flat {
			v, has := f[c]
			if !has || !usable(v) || seen[text(v)] {
				ok = false
				break
			}
			if _, isBool := v.(bool); isBool {
				ok = false
				break
			}
			seen[text(v)] = true
		}
		if ok {
			ids = append(ids, c)
		}
	}
	rank := func(c string) int {
		for i, p := range []string{"id", "name", "login", "title", "full_name", "slug", "number", "key", "tag_name"} {
			if c == p {
				return i
			}
		}
		if strings.HasSuffix(c, "id") || strings.HasSuffix(c, "name") {
			return 50
		}
		return 100 + strings.Count(c, ".")
	}
	slices.SortStableFunc(ids, func(a, b string) int { return cmp.Compare(rank(a), rank(b)) })
	if len(ids) == 0 {
		return ""
	}
	return ids[0]
}

// WritePayloadManifest records the payload Q&A set: each payload's source,
// checksum, table and questions with gold answers. Bodies are third-party
// content and are not included.
func WritePayloadManifest(path, codecName string, seed uint64, items []PayloadItem) error {
	type q struct {
		ID      string   `json:"id"`
		Kind    string   `json:"kind"`
		Text    string   `json:"text"`
		Answers []string `json:"answers"`
	}
	type entry struct {
		PayloadItem
		Questions []q `json:"questions"`
	}
	m := struct {
		Codec string  `json:"codec"`
		Seed  uint64  `json:"seed"`
		Rule  string  `json:"rule"`
		Items []entry `json:"items"`
	}{Codec: codecName, Seed: seed, Rule: fmt.Sprintf("largest array of objects cut to its first %d rows, halved until <= %d o200k tokens (min %d rows); kept only if the default gates encode it; up to %d questions per payload",
		payloadRows, payloadMaxTokens, payloadMinRows, payloadMaxQuestions)}
	for _, it := range items {
		e := entry{PayloadItem: it}
		for _, x := range it.Questions {
			e.Questions = append(e.Questions, q{x.ID, x.Kind, x.Text, x.Answers})
		}
		m.Items = append(m.Items, e)
	}
	b, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(b, '\n'), 0o644)
}
