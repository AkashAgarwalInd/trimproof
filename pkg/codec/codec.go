// Package codec defines the pluggable representation interface (spec §4, §6)
// and the structural eligibility rules shared by all codecs (Gate 1).
package codec

import (
	"errors"
	"fmt"
	"sort"
	"sync"
)

// SchemaMode controls how rows with differing key sets are handled.
type SchemaMode int

const (
	// Strict requires every row to have the same key set. Lossless.
	Strict SchemaMode = iota
	// Union fills missing keys with null. Lossy: missing and null become
	// indistinguishable, so it requires explicit LossyOptimizationPolicy.
	Union
)

func (m SchemaMode) String() string {
	if m == Union {
		return "UNION"
	}
	return "STRICT"
}

// Options configures a single Encode call.
type Options struct {
	Mode SchemaMode
}

// Codec converts canonical JSON to a token-efficient representation.
//
// Check is a cheap pre-filter; Encode is authoritative and may still return
// an error wrapping ErrIneligible. Contract: whenever Encode succeeds in a
// lossless mode, Decode(Encode(x)) == canonical(x), byte for byte.
type Codec interface {
	Name() string
	Version() string
	Lossless(mode SchemaMode) bool
	// Check is the codec-specific part of Gate 1. It reports why the value
	// is ineligible, or nil.
	Check(v any, opts Options) error
	Encode(canonicalJSON []byte, opts Options) ([]byte, error)
	Decode(encoded []byte) ([]byte, error)
	// Primer is the instruction text injected once per request when this
	// codec is used. Its token cost counts against Gate 4.
	Primer() string
}

// ErrIneligible wraps all Gate 1 rejections.
var ErrIneligible = errors.New("ineligible")

func ineligible(format string, args ...any) error {
	return fmt.Errorf("%w: %s", ErrIneligible, fmt.Sprintf(format, args...))
}

var (
	mu       sync.RWMutex
	registry = map[string]Codec{}
)

// Register makes a codec available by name. It panics on duplicates.
func Register(c Codec) {
	mu.Lock()
	defer mu.Unlock()
	if _, dup := registry[c.Name()]; dup {
		panic("codec: duplicate registration of " + c.Name())
	}
	registry[c.Name()] = c
}

// Get returns a registered codec.
func Get(name string) (Codec, bool) {
	mu.RLock()
	defer mu.RUnlock()
	c, ok := registry[name]
	return c, ok
}

// Names lists registered codecs in sorted order.
func Names() []string {
	mu.RLock()
	defer mu.RUnlock()
	out := make([]string, 0, len(registry))
	for n := range registry {
		out = append(out, n)
	}
	sort.Strings(out)
	return out
}
