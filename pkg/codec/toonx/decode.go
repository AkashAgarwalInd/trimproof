package toonx

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/AkashAgarwalInd/trimproof/pkg/canonical"
)

// Decode parses a toonx document and returns canonical JSON. It accepts
// only the forms the encoder writes.
func (Codec) Decode(encoded []byte) ([]byte, error) {
	return decode(encoded, &features{})
}

// features records which extensions to TOON a document uses.
type features struct{ absent, paths, json, list, consts, prefix, split bool }

func decode(encoded []byte, f *features) ([]byte, error) {
	d := &decoder{lines: strings.Split(string(encoded), "\n"), f: f}
	var v any
	var err error
	if strings.HasPrefix(d.lines[0], "[") {
		v, err = d.field(0, d.lines[0])
	} else {
		v, err = d.object(0)
	}
	if err == nil && d.pos != len(d.lines) {
		err = d.errf("unexpected content")
	}
	if err == nil {
		v, err = join(v, f)
	}
	if err != nil {
		return nil, err
	}
	return canonical.Marshal(v)
}

type decoder struct {
	lines []string
	pos   int
	f     *features
}

func (d *decoder) errf(format string, args ...any) error {
	return fmt.Errorf("toonx: line %d: %s", d.pos+1, fmt.Sprintf(format, args...))
}

// at returns the current line's content when it is indented to depth.
func (d *decoder) at(depth int) (string, bool) {
	if d.pos >= len(d.lines) {
		return "", false
	}
	l, pre := d.lines[d.pos], strings.Repeat("  ", depth)
	if !strings.HasPrefix(l, pre) || strings.HasPrefix(l[len(pre):], " ") {
		return "", false
	}
	return l[len(pre):], true
}

func (d *decoder) object(depth int) (map[string]any, error) {
	m := map[string]any{}
	for {
		line, ok := d.at(depth)
		if !ok || line == "" {
			break
		}
		key, rest, err := splitKey(line)
		if err != nil {
			return nil, d.errf("%v", err)
		}
		if _, dup := m[key]; dup {
			return nil, d.errf("duplicate key %q", key)
		}
		if m[key], err = d.field(depth, rest); err != nil {
			return nil, err
		}
	}
	if len(m) == 0 {
		return nil, d.errf("expected a field")
	}
	return m, nil
}

// field parses what follows a key on the current line, and any indented
// lines that belong to it.
func (d *decoder) field(depth int, rest string) (any, error) {
	if !strings.HasPrefix(rest, "[") {
		d.pos++
		switch {
		case rest == ":":
			return d.object(depth + 1)
		case rest == ": {}":
			return map[string]any{}, nil
		case strings.HasPrefix(rest, ": "):
			v, err := scalar(rest[2:])
			if err != nil {
				return nil, d.errf("%v", err)
			}
			return v, nil
		}
		return nil, d.errf("malformed field")
	}
	end := strings.IndexByte(rest, ']')
	if end < 0 {
		return nil, d.errf("malformed array header")
	}
	n, err := strconv.Atoi(rest[1:end])
	if err != nil || n < 0 || strconv.Itoa(n) != rest[1:end] {
		return nil, d.errf("bad array length")
	}
	rest = rest[end+1:]
	d.pos++
	switch {
	case rest == ":" && n == 0:
		return []any{}, nil
	case rest == ":":
		d.f.list = true
		out := make([]any, n)
		for i := range out {
			line, ok := d.at(depth + 1)
			if !ok || !strings.HasPrefix(line, "- ") {
				return nil, d.errf("expected list item")
			}
			if out[i], err = single(line[2:]); err != nil {
				return nil, d.errf("%v", err)
			}
			d.note(out[i])
			d.pos++
		}
		return out, nil
	case strings.HasPrefix(rest, ": ") && n > 0:
		vals, err := cells(rest[2:], n)
		if err != nil {
			return nil, d.errf("%v", err)
		}
		out := make([]any, n)
		for i, v := range vals {
			if v == absent {
				return nil, d.errf("empty array element")
			}
			if _, ok := v.(string); !ok && !isScalar(v) {
				return nil, d.errf("container in primitive array")
			}
			out[i] = v
		}
		return out, nil
	case strings.HasPrefix(rest, "{") && strings.HasSuffix(rest, "}:") && n > 0:
		cols, err := header(rest[1 : len(rest)-2])
		if err != nil {
			return nil, d.errf("%v", err)
		}
		for _, c := range cols {
			d.f.paths = d.f.paths || len(c) > 1
		}
		var consts []assignVal
		prefix := make([]string, len(cols))
		if line, ok := d.at(depth + 1); ok && strings.HasPrefix(line, "all rows: ") {
			if consts, err = assignments(line[len("all rows: "):]); err != nil {
				return nil, d.errf("%v", err)
			}
			d.f.consts = true
			d.pos++
		}
		if line, ok := d.at(depth + 1); ok && strings.HasPrefix(line, "starts with: ") {
			ps, err := assignments(line[len("starts with: "):])
			if err != nil {
				return nil, d.errf("%v", err)
			}
			for _, a := range ps {
				i := slices.IndexFunc(cols, func(c []string) bool { return slices.Equal(c, a.path) })
				p, isStr := a.val.(string)
				if i < 0 || !isStr || p == "" || prefix[i] != "" {
					return nil, d.errf("bad prefix")
				}
				prefix[i] = p
			}
			d.f.prefix = true
			d.pos++
		}
		all := slices.Clone(cols)
		for _, a := range consts {
			all = append(all, a.path)
		}
		out := make([]any, n)
		for i := range out {
			line, ok := d.at(depth + 1)
			if !ok {
				return nil, d.errf("expected table row")
			}
			vals, err := cells(line, len(cols))
			if err != nil {
				return nil, d.errf("%v", err)
			}
			for j, v := range vals {
				d.note(v)
				if prefix[j] == "" || v == absent || v == nil {
					continue
				}
				s, ok := v.(string)
				if !ok {
					return nil, d.errf("non-string cell in a prefixed column")
				}
				vals[j] = prefix[j] + s
			}
			for _, a := range consts {
				vals = append(vals, a.val)
			}
			if out[i], err = row(all, vals); err != nil {
				return nil, d.errf("%v", err)
			}
			d.pos++
		}
		return out, nil
	}
	return nil, d.errf("malformed array header")
}

// note records the extensions a table cell or list item uses.
func (d *decoder) note(v any) {
	switch v.(type) {
	case []any, map[string]any:
		d.f.json = true
	case struct{}:
		d.f.absent = true
	}
}

// absent marks an empty table cell.
var absent any = struct{}{}

func isScalar(v any) bool {
	switch v.(type) {
	case nil, bool, json.Number:
		return true
	}
	return false
}

// splitKey splits a field line into its key and the rest, which starts with
// ':' or '['.
func splitKey(line string) (string, string, error) {
	if strings.HasPrefix(line, `"`) {
		n, err := quotedLen(line)
		if err != nil {
			return "", "", err
		}
		var k string
		if err := json.Unmarshal([]byte(line[:n]), &k); err != nil {
			return "", "", err
		}
		if bareKey(k, true) {
			return "", "", errors.New("needlessly quoted key")
		}
		return k, line[n:], nil
	}
	i := strings.IndexAny(line, ":[")
	if i <= 0 || !bareKey(line[:i], true) {
		return "", "", errors.New("malformed key")
	}
	return line[:i], line[i:], nil
}

// quotedLen returns the length of the quoted string at the start of s.
func quotedLen(s string) (int, error) {
	for i := 1; i < len(s); i++ {
		switch s[i] {
		case '\\':
			i++
		case '"':
			return i + 1, nil
		}
	}
	return 0, errors.New("unterminated string")
}

// header parses table columns: paths of bare or quoted segments joined by
// dots, separated by commas.
func header(s string) ([][]string, error) {
	var cols [][]string
	seen := map[string]bool{}
	for i := 0; ; {
		path, next, err := columnPathAt(s, i)
		if err != nil {
			return nil, err
		}
		id := strings.Join(path, "\x00")
		if seen[id] {
			return nil, errors.New("duplicate column")
		}
		seen[id] = true
		cols, i = append(cols, path), next
		if i == len(s) {
			return cols, nil
		}
		if s[i] != ',' {
			return nil, errors.New("malformed header")
		}
		i++
	}
}

// columnPathAt parses one column path at s[i:] and returns it with the
// index after it.
func columnPathAt(s string, i int) ([]string, int, error) {
	var path []string
	for {
		var seg string
		if i < len(s) && s[i] == '"' {
			n, err := quotedLen(s[i:])
			if err != nil {
				return nil, 0, err
			}
			if err := json.Unmarshal([]byte(s[i:i+n]), &seg); err != nil {
				return nil, 0, err
			}
			if bareKey(seg, false) {
				return nil, 0, errors.New("needlessly quoted column")
			}
			i += n
		} else {
			j := i
			for j < len(s) && s[j] != '.' && s[j] != ',' && s[j] != '=' {
				j++
			}
			seg = s[i:j]
			if !bareKey(seg, false) {
				return nil, 0, fmt.Errorf("malformed column %q", seg)
			}
			i = j
		}
		path = append(path, seg)
		if i < len(s) && s[i] == '.' {
			i++
			continue
		}
		return path, i, nil
	}
}

// assignVal is one "path=value" item of an "all rows:" or "starts with:"
// line.
type assignVal struct {
	path []string
	val  any
}

// assignments parses comma-separated path=value items.
func assignments(s string) ([]assignVal, error) {
	var out []assignVal
	for i := 0; ; {
		path, next, err := columnPathAt(s, i)
		if err != nil {
			return nil, err
		}
		if next == len(s) || s[next] != '=' {
			return nil, errors.New("expected '='")
		}
		v, next, err := cell(s, next+1)
		if err != nil {
			return nil, err
		}
		if v == absent {
			return nil, errors.New("empty value")
		}
		out = append(out, assignVal{path, v})
		if next == len(s) {
			return out, nil
		}
		if s[next] != ',' {
			return nil, errors.New("expected ','")
		}
		i = next + 1
	}
}

// cells parses exactly n comma-separated cells.
func cells(s string, n int) ([]any, error) {
	out := make([]any, 0, n)
	i := 0
	for {
		v, next, err := cell(s, i)
		if err != nil {
			return nil, err
		}
		out = append(out, v)
		if next == len(s) {
			break
		}
		if s[next] != ',' {
			return nil, errors.New("expected ','")
		}
		i = next + 1
	}
	if len(out) != n {
		return nil, fmt.Errorf("%d cells, want %d", len(out), n)
	}
	return out, nil
}

func cell(s string, i int) (any, int, error) {
	if i == len(s) || s[i] == ',' {
		return absent, i, nil
	}
	switch s[i] {
	case '"':
		n, err := quotedLen(s[i:])
		if err != nil {
			return nil, 0, err
		}
		v, err := scalar(s[i : i+n])
		return v, i + n, err
	case '[', '{':
		dec := json.NewDecoder(strings.NewReader(s[i:]))
		var raw json.RawMessage
		if err := dec.Decode(&raw); err != nil {
			return nil, 0, err
		}
		n := int(dec.InputOffset())
		v, err := canonical.Parse(raw)
		if err != nil {
			return nil, 0, err
		}
		if b, _ := canonical.Marshal(v); !bytes.Equal(b, raw) {
			return nil, 0, errors.New("JSON cell is not canonical")
		}
		return v, i + n, nil
	}
	j := strings.IndexByte(s[i:], ',')
	if j < 0 {
		j = len(s) - i
	}
	v, err := scalar(s[i : i+j])
	return v, i + j, err
}

// single parses one list item.
func single(s string) (any, error) {
	v, next, err := cell(s, 0)
	if err != nil {
		return nil, err
	}
	if v == absent || next != len(s) {
		return nil, errors.New("malformed list item")
	}
	return v, nil
}

// scalar parses a primitive token: a quoted string, true, false, null, a
// number literal, or a bare string that needs no quoting.
func scalar(s string) (any, error) {
	if strings.HasPrefix(s, `"`) {
		var str string
		if n, err := quotedLen(s); err != nil || n != len(s) {
			return nil, errors.New("malformed string")
		}
		if err := json.Unmarshal([]byte(s), &str); err != nil {
			return nil, err
		}
		if !needsQuoting(str) || quote(str) != s {
			return nil, errors.New("string is not in canonical quoting")
		}
		return str, nil
	}
	switch s {
	case "true":
		return true, nil
	case "false":
		return false, nil
	case "null":
		return nil, nil
	}
	if canonical.IsNumberLiteral(s) {
		return json.Number(s), nil
	}
	if needsQuoting(s) {
		return nil, fmt.Errorf("unquoted string %q", s)
	}
	return s, nil
}

// node builds one row from path cells, so that conflicting paths fail.
type node struct {
	leaf  any
	isSet bool
	kids  map[string]*node
}

func row(cols [][]string, vals []any) (map[string]any, error) {
	root := &node{kids: map[string]*node{}}
	for i, path := range cols {
		if vals[i] == absent {
			continue
		}
		n := root
		for j, seg := range path {
			if n.isSet {
				return nil, errors.New("path under a value")
			}
			if n.kids == nil {
				n.kids = map[string]*node{}
			}
			k, ok := n.kids[seg]
			if !ok {
				k = &node{}
				n.kids[seg] = k
			}
			if j == len(path)-1 {
				if k.isSet || k.kids != nil {
					return nil, errors.New("conflicting cells")
				}
				k.leaf, k.isSet = vals[i], true
			}
			n = k
		}
	}
	if len(root.kids) == 0 {
		return nil, errors.New("empty row")
	}
	return root.value().(map[string]any), nil
}

func (n *node) value() any {
	if n.isSet {
		return n.leaf
	}
	m := make(map[string]any, len(n.kids))
	for k, c := range n.kids {
		m[k] = c.value()
	}
	return m
}
