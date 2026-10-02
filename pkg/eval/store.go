package eval

import (
	"bufio"
	"encoding/json"
	"os"
	"sync"
	"time"

	"github.com/AkashAgarwalInd/trimproof/pkg/ir"
)

// PairKind distinguishes treatment pairs from control (noise-floor) pairs.
type PairKind string

const (
	Treatment PairKind = "TREATMENT" // JSON arm vs codec arm
	Control   PairKind = "CONTROL"   // JSON arm vs a second JSON arm
)

// EvaluationPair is one paired observation (spec §8.3).
type EvaluationPair struct {
	ID           string    `json:"id"`
	Time         time.Time `json:"time"`
	Kind         PairKind  `json:"kind"`
	TenantID     string    `json:"tenant_id"`
	RouteID      string    `json:"route_id"`
	Model        string    `json:"model"`
	Codec        string    `json:"codec"`
	CodecVersion string    `json:"codec_version"`
	Agreement    Agreement `json:"agreement"`
	UsageA       ir.Usage  `json:"usage_a"`
	UsageB       ir.Usage  `json:"usage_b"`
	// Tier1A/B are nil when the route has no Tier 1 validator.
	Tier1A *bool  `json:"tier1_a,omitempty"`
	Tier1B *bool  `json:"tier1_b,omitempty"`
	ErrA   string `json:"err_a,omitempty"`
	ErrB   string `json:"err_b,omitempty"`
}

// Valid reports whether both arms produced a response.
func (p EvaluationPair) Valid() bool { return p.ErrA == "" && p.ErrB == "" }

// Store persists pairs.
type Store interface {
	Add(p EvaluationPair) error
	// Pairs returns pairs for a route, oldest first.
	Pairs(tenant, route string) []EvaluationPair
}

// MemoryStore keeps pairs in memory, optionally appending them to a JSONL
// file so CLI reports can read them.
type MemoryStore struct {
	mu    sync.RWMutex
	pairs map[string][]EvaluationPair
	f     *os.File
	w     *bufio.Writer
}

// NewMemoryStore returns a store; path may be empty. Existing pairs in
// path are loaded.
func NewMemoryStore(path string) (*MemoryStore, error) {
	s := &MemoryStore{pairs: map[string][]EvaluationPair{}}
	if path == "" {
		return s, nil
	}
	if prev, err := ReadPairs(path); err == nil {
		for _, p := range prev {
			k := p.TenantID + "\x00" + p.RouteID
			s.pairs[k] = append(s.pairs[k], p)
		}
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, err
	}
	s.f, s.w = f, bufio.NewWriter(f)
	return s, nil
}

func (s *MemoryStore) Add(p EvaluationPair) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	k := p.TenantID + "\x00" + p.RouteID
	s.pairs[k] = append(s.pairs[k], p)
	if s.w != nil {
		b, err := json.Marshal(p)
		if err != nil {
			return err
		}
		s.w.Write(append(b, '\n'))
		return s.w.Flush()
	}
	return nil
}

func (s *MemoryStore) Pairs(tenant, route string) []EvaluationPair {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return append([]EvaluationPair(nil), s.pairs[tenant+"\x00"+route]...)
}

// Close flushes and closes the backing file.
func (s *MemoryStore) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.f == nil {
		return nil
	}
	s.w.Flush()
	return s.f.Close()
}

// ReadPairs loads a JSONL pair file.
func ReadPairs(path string) ([]EvaluationPair, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var out []EvaluationPair
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 16<<20)
	for sc.Scan() {
		var p EvaluationPair
		if err := json.Unmarshal(sc.Bytes(), &p); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, sc.Err()
}
