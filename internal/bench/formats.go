package bench

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"fmt"

	"github.com/AkashAgarwalInd/context-mesh/pkg/canonical"
	"github.com/AkashAgarwalInd/context-mesh/pkg/codec"
	_ "github.com/AkashAgarwalInd/context-mesh/pkg/codec/tabular"
	_ "github.com/AkashAgarwalInd/context-mesh/pkg/codec/toon"
)

// Formats in report order. json-compact is the baseline; json-pretty is
// shown only to explain why published headline savings look larger.
var Formats = []string{"json-pretty", "json-compact", "toon", "tabular", "csv"}

// LiveFormats are sent to models. json-compact-2 is the control arm: the
// baseline sent a second time to measure the noise floor.
var LiveFormats = []string{"json-compact", "json-compact-2", "toon", "tabular", "csv"}

const csvPrimer = `Some tool results are CSV: the first line is the column names and each following line is one record.`

// Render returns the tool result text for format and the primer (if any)
// that goes into the system prompt.
func Render(format string, d *Dataset) (string, string, error) {
	raw := d.JSON()
	canon, err := canonical.Canonicalize(raw)
	if err != nil {
		return "", "", err
	}
	switch format {
	case "json-compact", "json-compact-2":
		return string(canon), "", nil
	case "json-pretty":
		var buf bytes.Buffer
		if err := json.Indent(&buf, canon, "", "  "); err != nil {
			return "", "", err
		}
		return buf.String(), "", nil
	case "toon", "tabular":
		c, _ := codec.Get(format)
		enc, err := c.Encode(canon, codec.Options{Mode: codec.Strict})
		if err != nil {
			return "", "", fmt.Errorf("%s/%s: %w", d.Name, format, err)
		}
		return string(enc), c.Primer(), nil
	case "csv":
		v, _ := canonical.Parse(canon)
		t, err := codec.AsTable(v, codec.Strict, true)
		if err != nil {
			return "", "", err
		}
		var buf bytes.Buffer
		w := csv.NewWriter(&buf)
		_ = w.Write(t.Columns)
		for _, row := range t.Rows {
			rec := make([]string, len(t.Columns))
			for i, c := range t.Columns {
				switch x := row[c].(type) {
				case nil:
					rec[i] = ""
				case string:
					rec[i] = x
				default:
					b, _ := canonical.Marshal(x)
					rec[i] = string(b)
				}
			}
			_ = w.Write(rec)
		}
		w.Flush()
		return string(bytes.TrimRight(buf.Bytes(), "\n")), csvPrimer, w.Error()
	}
	return "", "", fmt.Errorf("unknown format %q", format)
}
