// Package toonx is TOON with extensions that let real API responses become
// tables, where plain TOON falls back to a list form that is often larger
// than compact JSON:
//
//   - every array of objects is a table, also when rows have different keys:
//     an empty cell means the key is absent (an empty string is "");
//   - nested objects in table rows may be flattened into path columns, so
//     {"user":{"id":1}} becomes column user.id; a key that itself contains a
//     dot is quoted, so a bare dotted column is always a path;
//   - a table cell or list item that holds an array or an empty object is
//     written as compact JSON, starting with [ or {;
//   - a field with the same value in every row is written once, on an
//     "all rows: a=x" line under the header, instead of in each row;
//   - in a column of URLs, a shared start is written once, on a
//     "starts with: a=p" line, and left out of the column's strings;
//   - an array of objects with more than 8 varying columns is split into
//     tables part1, part2, … of at most 8 columns, each starting with the
//     row number "#". On wider tables models count commas to find a column,
//     which costs them thousands of output tokens.
//
// The "all rows" and "starts with" lines apply only when they make the table
// shorter. Neither line can be read as a row: an unquoted cell never holds a
// colon.
//
// On a flat array of uniform objects with at most 8 columns, no constant
// field and no URL column the output is the same as TOON's. The
// encoder and decoder are trimproof's own; every Encode decodes its output
// again and fails with codec.ErrIneligible unless the round trip is exact,
// so number literals and strings are kept byte for byte.
package toonx

import (
	"bytes"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"unicode"

	"github.com/AkashAgarwalInd/trimproof/pkg/canonical"
	"github.com/AkashAgarwalInd/trimproof/pkg/codec"
	"github.com/AkashAgarwalInd/trimproof/pkg/codec/toon"
)

// Primer sentences beyond TOON's, each sent only when a request uses it.
const (
	primerAbsent = ` In an array of objects, an empty value means the field is absent.`
	primerPaths  = ` A field named a.b is field b of the nested object a.`
	primerJSON   = ` A value starting with [ or { is JSON.`
	primerList   = ` "key[N]:" followed by "- " lines is a list of N items.`
	primerConst  = ` A line "all rows: a=x" under a table header means every row also has field a with value x.`
	primerPrefix = ` A line "starts with: a=p" under a table header means every text value of field a starts with p, which is left out of the rows: put p back in front.`
	primerSplit  = ` A wide array may be split into tables part1, part2 and so on: they hold the same rows in the same order, and column # is the row number.`
)

// Codec is the toonx codec. The zero value is ready to use.
type Codec struct{}

func init() { codec.Register(Codec{}) }

func (Codec) Name() string    { return "toonx" }
func (Codec) Version() string { return "3" }

// Primer is TOON's primer with every extension described.
func (Codec) Primer() string {
	return toon.Codec{}.Primer() + primerAbsent + primerPaths + primerJSON + primerList + primerConst + primerPrefix + primerSplit
}

// PrimerFor implements codec.DynamicPrimer: TOON's primer, plus only the
// extensions the encoded blocks use. For flat tables it is TOON's primer.
func (c Codec) PrimerFor(encoded [][]byte) string {
	return c.PrimerWith(toon.Codec{}.Primer(), encoded)
}

// PrimerWith is PrimerFor with base in place of TOON's primer text, for
// testing other wordings.
func (c Codec) PrimerWith(base string, encoded [][]byte) string {
	var f features
	for _, b := range encoded {
		if _, err := decode(b, &f); err != nil {
			f = features{true, true, true, true, true, true, true}
			break
		}
	}
	p := base
	for _, x := range []struct {
		used bool
		text string
	}{{f.absent, primerAbsent}, {f.paths, primerPaths}, {f.json, primerJSON}, {f.list, primerList},
		{f.consts, primerConst}, {f.prefix, primerPrefix}, {f.split, primerSplit}} {
		if x.used {
			p += x.text
		}
	}
	return p
}

// Lossless is true under Strict. Absent keys stay absent, so toonx never
// needs Union's null filling.
func (Codec) Lossless(mode codec.SchemaMode) bool { return mode == codec.Strict }

// Check applies Gate 1: any non-empty JSON object or array.
func (Codec) Check(v any, _ codec.Options) error {
	switch t := v.(type) {
	case []any:
		if len(t) == 0 {
			return fmt.Errorf("%w: empty array", codec.ErrIneligible)
		}
	case map[string]any:
		if len(t) == 0 {
			return fmt.Errorf("%w: empty object", codec.ErrIneligible)
		}
	default:
		return fmt.Errorf("%w: top-level value is not an object or array", codec.ErrIneligible)
	}
	return nil
}

func (c Codec) Encode(canonicalJSON []byte, opts codec.Options) ([]byte, error) {
	v, err := canonical.Parse(canonicalJSON)
	if err != nil {
		return nil, err
	}
	if err := c.Check(v, opts); err != nil {
		return nil, err
	}
	want, err := canonical.Marshal(v)
	if err != nil {
		return nil, err
	}
	return c.EncodeValue(v, want, opts)
}

// EncodeValue implements codec.ValueEncoder.
func (c Codec) EncodeValue(v any, canonicalJSON []byte, _ codec.Options) ([]byte, error) {
	e := &encoder{}
	v, _ = split(v)
	switch t := v.(type) {
	case map[string]any:
		e.object(0, t)
	case []any:
		e.array(0, "", t)
	}
	enc := bytes.TrimSuffix(e.buf.Bytes(), []byte("\n"))
	got, err := c.Decode(enc)
	if err != nil || string(got) != string(canonicalJSON) {
		return nil, fmt.Errorf("%w: toonx round trip failed", codec.ErrIneligible)
	}
	return enc, nil
}

type encoder struct {
	buf bytes.Buffer
}

func (e *encoder) line(depth int, parts ...string) {
	for range depth {
		e.buf.WriteString("  ")
	}
	for _, p := range parts {
		e.buf.WriteString(p)
	}
	e.buf.WriteByte('\n')
}

func (e *encoder) object(depth int, m map[string]any) {
	for _, k := range canonical.SortedKeys(m) {
		key := fieldKey(k)
		switch t := m[k].(type) {
		case map[string]any:
			if len(t) == 0 {
				e.line(depth, key, ": {}")
				continue
			}
			e.line(depth, key, ":")
			e.object(depth+1, t)
		case []any:
			e.array(depth, key, t)
		default:
			e.line(depth, key, ": ", primitive(t))
		}
	}
}

func (e *encoder) array(depth int, key string, a []any) {
	n := strconv.Itoa(len(a))
	if len(a) == 0 {
		e.line(depth, key, "[0]:")
		return
	}
	if primitives(a) {
		cells := make([]string, len(a))
		for i, x := range a {
			cells[i] = primitive(x)
		}
		e.line(depth, key, "[", n, "]: ", strings.Join(cells, ","))
		return
	}
	if t, ok := newTable(a); ok {
		e.line(depth, key, "[", n, "]{", t.header(), "}:")
		for _, l := range t.extra() {
			e.line(depth+1, l)
		}
		for _, row := range t.rows {
			e.line(depth+1, row)
		}
		return
	}
	e.line(depth, key, "[", n, "]:")
	for _, x := range a {
		e.line(depth+1, "- ", cellValue(x))
	}
}

func primitives(a []any) bool {
	for _, x := range a {
		switch x.(type) {
		case []any, map[string]any:
			return false
		}
	}
	return true
}

// table is an array of objects as a header of column paths, the fields
// every row shares, the URL prefixes of columns, and one line of cells per
// row.
type table struct {
	cols     [][]string
	consts   []assign // fields with one value in every row, not in cols
	prefixes []assign // shared starts of columns' strings
	rows     []string
}

// assign is one "path=value" item of an "all rows:" or "starts with:" line.
type assign struct {
	path []string
	val  string // the cell as written
}

// newTable lays a out as a table, flattened or not, with constant fields and
// URL prefixes factored out or not, whichever is shortest. ok is false when
// a holds anything but non-empty objects.
func newTable(a []any) (*table, bool) {
	rows := make([]map[string]any, len(a))
	for i, x := range a {
		m, ok := x.(map[string]any)
		if !ok || len(m) == 0 {
			return nil, false
		}
		rows[i] = m
	}
	var best *table
	for _, flatten := range []bool{false, true} {
		for _, factor := range []bool{false, true} {
			if t := layout(rows, flatten, factor); best == nil || t.size() < best.size() {
				best = t
			}
		}
	}
	return best, true
}

func layout(rows []map[string]any, flatten, factor bool) *table {
	cells := make([]map[string]any, len(rows))
	paths := map[string][]string{}
	for i, r := range rows {
		cells[i] = map[string]any{}
		collect(nil, r, flatten, cells[i], paths)
	}
	ids := make([]string, 0, len(paths))
	for id := range paths {
		ids = append(ids, id)
	}
	slices.Sort(ids)
	t := &table{}
	prefix := map[string]string{}
	if factor {
		var kept []string
		for _, id := range ids {
			if v, ok := constant(cells, id); ok {
				t.consts = append(t.consts, assign{paths[id], cellValue(v)})
				continue
			}
			kept = append(kept, id)
		}
		if len(kept) == 0 { // a table needs a column; keep the last
			t.consts, kept = t.consts[:len(t.consts)-1], ids[len(ids)-1:]
		}
		ids = kept
		for _, id := range ids {
			if p := urlPrefix(cells, id); p != "" {
				prefix[id] = p
				t.prefixes = append(t.prefixes, assign{paths[id], primitive(p)})
			}
		}
	}
	for _, id := range ids {
		t.cols = append(t.cols, paths[id])
	}
	for _, c := range cells {
		vals := make([]string, len(ids))
		for i, id := range ids {
			v, ok := c[id]
			if !ok {
				continue
			}
			if s, isStr := v.(string); isStr && prefix[id] != "" {
				v = strings.TrimPrefix(s, prefix[id])
			}
			vals[i] = cellValue(v)
		}
		t.rows = append(t.rows, strings.Join(vals, ","))
	}
	return t
}

// minFactorRows is the fewest rows a table needs before constant fields
// or URL prefixes are factored out.
const minFactorRows = 3

// constant returns the value of column id when every row has it and it is
// the same in all of them.
func constant(cells []map[string]any, id string) (any, bool) {
	first, ok := cells[0][id]
	if !ok || len(cells) < minFactorRows {
		return nil, false
	}
	want, _ := canonical.Marshal(first)
	for _, c := range cells[1:] {
		v, ok := c[id]
		if !ok {
			return nil, false
		}
		if b, _ := canonical.Marshal(v); string(b) != string(want) {
			return nil, false
		}
	}
	return first, true
}

// urlPrefix returns the start shared by every string of column id, cut
// after its last '/', when the table has minFactorRows rows, the column
// holds only strings, null or absent cells, at least two strings, and the
// start is an http(s) URL with a path beyond the scheme. No string's rest
// is only digits. Otherwise it returns "".
func urlPrefix(cells []map[string]any, id string) string {
	if len(cells) < minFactorRows {
		return ""
	}
	var strs []string
	for _, c := range cells {
		switch v := c[id].(type) {
		case string:
			strs = append(strs, v)
		case nil:
		default:
			return ""
		}
	}
	if len(strs) < 2 {
		return ""
	}
	p := strs[0]
	for _, s := range strs[1:] {
		n := 0
		for n < len(p) && n < len(s) && p[n] == s[n] {
			n++
		}
		p = p[:n]
	}
	scheme := strings.Index(p, "://")
	if !strings.HasPrefix(p, "http") || scheme < 0 {
		return ""
	}
	cut := strings.LastIndexByte(p, '/') + 1
	// A rest that is all digits would read as a number (an id); cut one
	// segment earlier so it keeps its path, as in shows/266.
	for cut > scheme+3 && slices.ContainsFunc(strs, func(s string) bool { return digits(s[cut:]) }) {
		cut = strings.LastIndexByte(p[:cut-1], '/') + 1
	}
	if cut <= scheme+3 {
		return ""
	}
	return p[:cut]
}

// digits reports whether s is non-empty and only ASCII digits.
func digits(s string) bool {
	return s != "" && strings.Trim(s, "0123456789") == ""
}

// collect adds m's leaves to out under their path IDs. With flatten, a
// non-empty nested object contributes its own leaves instead of one cell.
func collect(prefix []string, m map[string]any, flatten bool, out map[string]any, paths map[string][]string) {
	for k, v := range m {
		p := append(slices.Clip(prefix), k)
		if sub, ok := v.(map[string]any); ok && flatten && len(sub) > 0 {
			collect(p, sub, true, out, paths)
			continue
		}
		id := strings.Join(p, "\x00")
		out[id] = v
		paths[id] = p
	}
}

func columnPath(p []string) string {
	segs := make([]string, len(p))
	for j, s := range p {
		segs[j] = columnKey(s)
	}
	return strings.Join(segs, ".")
}

func (t *table) header() string {
	cols := make([]string, len(t.cols))
	for i, p := range t.cols {
		cols[i] = columnPath(p)
	}
	return strings.Join(cols, ",")
}

// extra returns the "all rows:" and "starts with:" lines, if any.
func (t *table) extra() []string {
	var out []string
	for _, x := range []struct {
		label string
		as    []assign
	}{{"all rows: ", t.consts}, {"starts with: ", t.prefixes}} {
		if len(x.as) == 0 {
			continue
		}
		items := make([]string, len(x.as))
		for i, a := range x.as {
			items[i] = columnPath(a.path) + "=" + a.val
		}
		out = append(out, x.label+strings.Join(items, ","))
	}
	return out
}

func (t *table) size() int {
	n := len(t.header())
	for _, l := range t.extra() {
		n += len(l) + 1
	}
	for _, r := range t.rows {
		n += len(r) + 1
	}
	return n
}

// cellValue writes a table cell or list item: a primitive, or compact JSON
// for an array or object.
func cellValue(v any) string {
	switch v.(type) {
	case []any, map[string]any:
		b, _ := canonical.Marshal(v)
		return string(b)
	}
	return primitive(v)
}

// primitive writes a scalar with TOON's quoting rules. Strings holding a
// comma are quoted everywhere: it is TOON's default delimiter both in arrays
// and in the document.
func primitive(v any) string {
	switch t := v.(type) {
	case nil:
		return "null"
	case bool:
		return strconv.FormatBool(t)
	case string:
		if needsQuoting(t) {
			return quote(t)
		}
		return t
	default: // json.Number, already a valid literal
		return fmt.Sprint(t)
	}
}

func quote(s string) string {
	var b bytes.Buffer
	canonical.AppendString(&b, s)
	return b.String()
}

// needsQuoting follows TOON's rules (comma delimiter), plus every string
// that has a control character, which TOON cannot write at all.
func needsQuoting(s string) bool {
	if s == "" || strings.TrimSpace(s) != s {
		return true
	}
	switch s {
	case "true", "false", "null":
		return true
	}
	if looksNumeric(s) || (len(s) > 1 && s[0] == '0' && s[1] >= '0' && s[1] <= '9') {
		return true
	}
	if strings.ContainsAny(s, ":\\\"[]{},") || strings.HasPrefix(s, "-") {
		return true
	}
	for _, r := range s {
		if r < 0x20 {
			return true
		}
	}
	return false
}

func looksNumeric(s string) bool {
	i := 0
	if i < len(s) && s[i] == '-' {
		i++
	}
	digits := 0
	for i < len(s) && isDigit(s[i]) {
		i++
		digits++
	}
	if digits == 0 {
		return false
	}
	if i < len(s) && s[i] == '.' {
		i++
		if i == len(s) || !isDigit(s[i]) {
			return false
		}
		for i < len(s) && isDigit(s[i]) {
			i++
		}
	}
	if i < len(s) && (s[i] == 'e' || s[i] == 'E') {
		i++
		if i < len(s) && (s[i] == '+' || s[i] == '-') {
			i++
		}
		if i == len(s) || !isDigit(s[i]) {
			return false
		}
		for i < len(s) && isDigit(s[i]) {
			i++
		}
	}
	return i == len(s)
}

func isDigit(b byte) bool { return b >= '0' && b <= '9' }

// fieldKey writes an object field key as TOON does: bare when it is an
// identifier (dots allowed), else quoted.
func fieldKey(k string) string {
	if bareKey(k, true) {
		return k
	}
	return quote(k)
}

// columnKey writes one segment of a table column path. Dots are not
// allowed bare, so a bare dotted column is always a path.
func columnKey(k string) string {
	if bareKey(k, false) {
		return k
	}
	return quote(k)
}

func bareKey(k string, dots bool) bool {
	if k == "" {
		return false
	}
	for i, r := range k {
		switch {
		case r == '_' || unicode.IsLetter(r):
		case i > 0 && (unicode.IsDigit(r) || (dots && r == '.')):
		default:
			return false
		}
	}
	return true
}
