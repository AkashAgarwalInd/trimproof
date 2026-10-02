package bench

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"github.com/AkashAgarwalInd/trimproof/internal/bench/wtq"
)

// WTQDatasets turns a WikiTableQuestions sample into datasets: one table
// and one question each, the table sent in its source column order and the
// reply scored with the WTQ evaluator.
func WTQDatasets(items []wtq.Item) []*Dataset {
	out := make([]*Dataset, 0, len(items))
	for _, it := range items {
		it := it
		out = append(out, &Dataset{
			ID: "wtq-" + it.ID, Name: "wtq", Rows: it.Rows,
			Tool: "lookup_table", ToolArgs: "{}", Raw: it.JSON(),
			Questions: []Question{{
				ID: it.ID, Kind: "wtq", Text: it.Question, Answer: strings.Join(it.Answers, " | "),
				Answers: it.Answers, Check: it.Correct,
			}},
		})
	}
	return out
}

// WriteWTQManifest records exactly which WikiTableQuestions items a run
// used, with each source table's SHA-256, so the sample can be audited and
// rebuilt from the official release without redistributing it.
func WriteWTQManifest(path, dir string, items []wtq.Item, n, minRows int, seed uint64) error {
	type entry struct {
		ID          string   `json:"id"`
		Question    string   `json:"question"`
		Answers     []string `json:"answers"`
		Table       string   `json:"table"`
		TableSHA256 string   `json:"table_sha256"`
		Rows        int      `json:"rows"`
		Columns     []string `json:"columns"`
		JSONBytes   int      `json:"json_bytes"`
	}
	m := struct {
		Source  string  `json:"source"`
		License string  `json:"license"`
		Release string  `json:"release"`
		SHA256  string  `json:"release_sha256"`
		Split   string  `json:"split"`
		MinRows int     `json:"min_rows"`
		N       int     `json:"n"`
		Seed    uint64  `json:"seed"`
		Items   []entry `json:"items"`
	}{
		Source:  "WikiTableQuestions, Panupong Pasupat and Percy Liang (ACL 2015)",
		License: "CC BY-SA 4.0; the questions and answers quoted here are under that licence",
		Release: wtq.ReleaseURL, SHA256: wtq.ReleaseSHA256, Split: "pristine-unseen-tables",
		MinRows: minRows, N: n, Seed: seed,
	}
	for _, it := range items {
		b, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(it.TablePath)))
		if err != nil {
			return err
		}
		sum := sha256.Sum256(b)
		m.Items = append(m.Items, entry{it.ID, it.Question, it.Answers, it.TablePath, hex.EncodeToString(sum[:]),
			it.Rows, it.Columns, len(it.JSON())})
	}
	b, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(b, '\n'), 0o644)
}
