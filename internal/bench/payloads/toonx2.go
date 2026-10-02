package payloads

import (
	"github.com/AkashAgarwalInd/trimproof/pkg/codec"
	"github.com/AkashAgarwalInd/trimproof/pkg/codec/toonx"
)

// toonx2 is toonx version 2 (no split of wide tables) under its own name.
// Its gates chose the registered payload Q&A set, which stays fixed now
// that toonx is version 3 (Amendment 6).
type toonx2 struct{ toonx.Codec }

func (toonx2) Name() string    { return "toonx2" }
func (toonx2) Version() string { return "2" }

func init() { codec.Register(toonx2{toonx.Codec{NoSplit: true}}) }
