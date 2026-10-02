// Package ir is the provider-neutral request/response model (spec §4).
//
// Adapters parse wire requests into IR for analysis only. Rendering patches
// the original wire body in place, so fields the IR does not model survive,
// and an untransformed request is forwarded byte-identical (Request.Raw).
package ir

import "encoding/json"

// BlockKind classifies a content block.
type BlockKind int

const (
	Text BlockKind = iota
	ToolResult
	ToolUse
	MarkedData
	Other // images, documents, thinking, etc.; never transformed
)

func (k BlockKind) String() string {
	return [...]string{"text", "tool_result", "tool_use", "marked_data", "other"}[k]
}

// Locator addresses a block in the wire body: message index and content
// part index. Part is -1 when the message content is a plain string.
type Locator struct {
	Message int
	Part    int
}

// AppliedTransform records a codec applied to a data block.
type AppliedTransform struct {
	Codec   string
	Version string
	Encoded []byte
}

// Block is one piece of message content.
type Block struct {
	Kind      BlockKind
	Loc       Locator
	Text      string // text content (Text, ToolResult, MarkedData)
	ToolName  string // ToolResult: resolved from the matching tool call
	ToolUseID string
	IsError   bool
	// Data is set for ToolResult and MarkedData blocks whose text parses as
	// JSON. It is the raw text, not yet canonicalized.
	Data      []byte
	Transform *AppliedTransform
}

// Message is one conversation turn.
type Message struct {
	Role   string
	Blocks []Block
}

// ToolDef is a tool declared in the request.
type ToolDef struct {
	Name   string
	Schema json.RawMessage
}

// Request is the provider-neutral view of a request.
type Request struct {
	Provider string
	Model    string
	Stream   bool
	System   []Block
	Messages []Message
	Tools    []ToolDef
	// StructuredOutput is true when the request asks for JSON output
	// (OpenAI response_format, Anthropic output_format).
	StructuredOutput bool
	// Raw is the original wire body.
	Raw []byte
	// Primer, if set at render time, is appended to the system prompt.
	Primer string
}

// DataBlocks returns pointers to every block eligible for the gates: tool
// results and marked blocks that carry JSON and are not errors.
func (r *Request) DataBlocks() []*Block {
	var out []*Block
	for i := range r.Messages {
		for j := range r.Messages[i].Blocks {
			b := &r.Messages[i].Blocks[j]
			if (b.Kind == ToolResult || b.Kind == MarkedData) && b.Data != nil && !b.IsError {
				out = append(out, b)
			}
		}
	}
	return out
}

// Transformed reports whether any block carries a transform.
func (r *Request) Transformed() bool {
	for _, b := range r.DataBlocks() {
		if b.Transform != nil {
			return true
		}
	}
	return false
}

// Mark converts the text block at loc into a MarkedData block (client
// opt-in for data outside tool results). It reports whether loc matched.
func (r *Request) Mark(loc Locator) bool {
	if loc.Message < 0 || loc.Message >= len(r.Messages) {
		return false
	}
	for j := range r.Messages[loc.Message].Blocks {
		b := &r.Messages[loc.Message].Blocks[j]
		if b.Loc == loc && b.Kind == Text {
			b.Kind = MarkedData
			if json.Valid([]byte(b.Text)) {
				b.Data = []byte(b.Text)
			}
			return true
		}
	}
	return false
}

// ToolCall is a tool invocation proposed by the model.
type ToolCall struct {
	ID   string
	Name string
	Args json.RawMessage // as sent by the provider; canonicalize before comparing
}

// Usage is provider-reported token accounting.
type Usage struct {
	InputTokens       int
	OutputTokens      int
	CachedInputTokens int
}

// Response is the provider-neutral view of a non-streaming response.
type Response struct {
	Text       string
	ToolCalls  []ToolCall
	Structured json.RawMessage // set when the request asked for JSON output
	StopReason string
	Usage      Usage
	Raw        []byte
}
