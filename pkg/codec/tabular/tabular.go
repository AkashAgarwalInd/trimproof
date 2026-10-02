// Package tabular implements the strict lossless tabular codec: a
// header line of sorted column names followed by one compact JSON array per
// row. Values are primitives with their exact canonical text.
package tabular

import (
	"bytes"
	"errors"
	"fmt"
	"slices"

	"github.com/AkashAgarwalInd/trimproof/pkg/canonical"
	"github.com/AkashAgarwalInd/trimproof/pkg/codec"
)

const primer = `Some tool results are encoded as tables to save space. A table's first line is a JSON array of column names; each following line is a JSON array of one row's values in the same column order. A null value means the field is null.`

// Codec is the tabular codec. The zero value is ready to use.
type Codec struct{}

func init() { codec.Register(Codec{}) }

func (Codec) Name() string    { return "tabular" }
func (Codec) Version() string { return "1" }
func (Codec) Primer() string  { return primer }

func (Codec) Lossless(mode codec.SchemaMode) bool { return mode == codec.Strict }

func (Codec) Check(v any, opts codec.Options) error {
	_, err := codec.AsTable(v, opts.Mode, true)
	return err
}

func (Codec) Encode(canonicalJSON []byte, opts codec.Options) ([]byte, error) {
	v, err := canonical.Parse(canonicalJSON)
	if err != nil {
		return nil, err
	}
	t, err := codec.AsTable(v, opts.Mode, true)
	if err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	buf.WriteByte('[')
	for i, c := range t.Columns {
		if i > 0 {
			buf.WriteByte(',')
		}
		canonical.AppendString(&buf, c)
	}
	buf.WriteByte(']')
	for _, row := range t.Rows {
		buf.WriteString("\n[")
		for i, c := range t.Columns {
			if i > 0 {
				buf.WriteByte(',')
			}
			// Missing keys (Union mode only) are written as null.
			if err := canonical.AppendValue(&buf, row[c], 1); err != nil {
				return nil, err
			}
		}
		buf.WriteByte(']')
	}
	return buf.Bytes(), nil
}

// Decode parses a tabular document and returns canonical JSON. It enforces
// the grammar: unique, non-empty, sorted headers; arity equal to the header;
// primitive values only; no trailing data.
func (Codec) Decode(encoded []byte) ([]byte, error) {
	lines := bytes.Split(encoded, []byte("\n"))
	for i, l := range lines {
		lines[i] = bytes.TrimSuffix(l, []byte("\r"))
	}
	if len(lines) < 2 {
		return nil, errors.New("tabular: need a header and at least one row")
	}
	hv, err := canonical.Parse(lines[0])
	if err != nil {
		return nil, fmt.Errorf("tabular: header: %w", err)
	}
	harr, ok := hv.([]any)
	if !ok || len(harr) == 0 {
		return nil, errors.New("tabular: header must be a non-empty array")
	}
	cols := make([]string, len(harr))
	for i, h := range harr {
		s, ok := h.(string)
		if !ok || s == "" {
			return nil, fmt.Errorf("tabular: header %d must be a non-empty string", i)
		}
		cols[i] = s
	}
	for i := 1; i < len(cols); i++ {
		if cols[i-1] >= cols[i] {
			return nil, errors.New("tabular: header must be sorted and unique")
		}
	}
	if !slices.IsSorted(cols) {
		return nil, errors.New("tabular: header must be sorted")
	}

	rows := make([]any, 0, len(lines)-1)
	for n, line := range lines[1:] {
		rv, err := canonical.Parse(line)
		if err != nil {
			return nil, fmt.Errorf("tabular: row %d: %w", n, err)
		}
		rarr, ok := rv.([]any)
		if !ok {
			return nil, fmt.Errorf("tabular: row %d is not an array", n)
		}
		if len(rarr) != len(cols) {
			return nil, fmt.Errorf("tabular: row %d has arity %d, want %d", n, len(rarr), len(cols))
		}
		obj := make(map[string]any, len(cols))
		for i, val := range rarr {
			if k := codec.KindOf(val); k == codec.KindArray || k == codec.KindObject {
				return nil, fmt.Errorf("tabular: row %d column %q is not a primitive", n, cols[i])
			}
			obj[cols[i]] = val
		}
		rows = append(rows, obj)
	}
	return canonical.Marshal(rows)
}
