// Package ir is the provider-neutral request/response model.
//
// Adapters parse wire requests into IR for analysis only. Rendering patches
// the original wire body in place, so fields the IR does not model survive,
// and an untransformed request is forwarded byte-identical (Request.Raw).
package ir

import (
	"bytes"
	"encoding/json"
	"sort"
	"strings"
)

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
	Data []byte
	// Span is set when the data is only part of Text (a fenced code block
	// found by DetectData). Several blocks can then share one Loc.
	Span      *Span
	Transform *AppliedTransform
}

// Span locates a data block inside its block's Text, as byte offsets.
type Span struct {
	Start, End int // the data
	// LabelStart and LabelEnd locate the fence's info string ("json"),
	// which is replaced by the codec name when the data is encoded. They
	// are equal when the fence has no info string.
	LabelStart, LabelEnd int
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

// RenderedText returns the text to send at loc: the encoded payload when a
// whole-text block there was transformed, or the original text with every
// transformed span replaced (and its fence relabelled with the codec name).
func (r *Request) RenderedText(loc Locator) string {
	var full string
	var spans []*Block
	for _, b := range r.DataBlocks() {
		if b.Loc != loc {
			continue
		}
		full = b.Text
		if b.Transform == nil {
			continue
		}
		if b.Span == nil {
			return string(b.Transform.Encoded)
		}
		spans = append(spans, b)
	}
	sort.Slice(spans, func(i, j int) bool { return spans[i].Span.Start < spans[j].Span.Start })
	var sb strings.Builder
	pos := 0
	for _, b := range spans {
		sp := b.Span
		if sp.LabelEnd > sp.LabelStart {
			sb.WriteString(full[pos:sp.LabelStart])
			sb.WriteString(b.Transform.Codec)
			pos = sp.LabelEnd
		}
		sb.WriteString(full[pos:sp.Start])
		sb.Write(b.Transform.Encoded)
		pos = sp.End
	}
	sb.WriteString(full[pos:])
	return sb.String()
}

// DetectData marks JSON data in user messages so the gates can consider it
// (a route opt-in). A text block is data when its whole text is a JSON
// array or object, or when it holds fenced code blocks labelled json (or
// unlabelled) whose content is one; each such fence becomes its own block.
// Text that is not data is left alone, and the gates still decide whether
// any marked block is worth encoding.
func (r *Request) DetectData() {
	for i := range r.Messages {
		m := &r.Messages[i]
		if m.Role != "user" {
			continue
		}
		var out []Block
		changed := false
		for _, b := range m.Blocks {
			if b.Kind != Text {
				out = append(out, b)
				continue
			}
			if isJSONData(b.Text) {
				b.Kind, b.Data = MarkedData, []byte(b.Text)
				out, changed = append(out, b), true
				continue
			}
			spans := fencedJSON(b.Text)
			if len(spans) == 0 {
				out = append(out, b)
				continue
			}
			for _, sp := range spans {
				nb := b
				nb.Kind, nb.Span, nb.Data = MarkedData, &sp, []byte(b.Text[sp.Start:sp.End])
				out = append(out, nb)
			}
			changed = true
		}
		if changed {
			m.Blocks = out
		}
	}
}

// isJSONData reports whether text is a JSON array or object.
func isJSONData(text string) bool {
	t := bytes.TrimSpace([]byte(text))
	return len(t) > 0 && (t[0] == '[' || t[0] == '{') && json.Valid(t)
}

// fencedJSON finds fenced code blocks (``` or longer) whose info string is
// empty or "json" and whose content is a JSON array or object.
func fencedJSON(text string) []Span {
	var out []Span
	lines := splitLines(text)
	for i := 0; i < len(lines); i++ {
		open := lines[i]
		ticks, info := fence(text[open[0]:open[1]])
		if ticks == 0 {
			continue
		}
		label := strings.TrimSpace(info)
		if label != "" && !strings.EqualFold(label, "json") {
			// Skip to this fence's end so its content is not misread.
			for i++; i < len(lines); i++ {
				if t, rest := fence(text[lines[i][0]:lines[i][1]]); t >= ticks && strings.TrimSpace(rest) == "" {
					break
				}
			}
			continue
		}
		for j := i + 1; j < len(lines); j++ {
			t, rest := fence(text[lines[j][0]:lines[j][1]])
			if t < ticks || strings.TrimSpace(rest) != "" {
				continue
			}
			start, end := lines[i][2], lines[j][0] // content: after the opening line, before the closing one
			if end > start && text[end-1] == '\n' {
				end--
				if end > start && text[end-1] == '\r' {
					end--
				}
			}
			if start <= end && isJSONData(text[start:end]) {
				sp := Span{Start: start, End: end}
				if label != "" {
					sp.LabelStart = open[0] + strings.Index(text[open[0]:open[1]], label)
					sp.LabelEnd = sp.LabelStart + len(label)
				}
				out = append(out, sp)
			}
			i = j
			break
		}
	}
	return out
}

// splitLines returns, per line, its start, its end without the line break,
// and the start of the next line.
func splitLines(text string) [][3]int {
	var out [][3]int
	for start := 0; start < len(text); {
		nl := strings.IndexByte(text[start:], '\n')
		if nl < 0 {
			out = append(out, [3]int{start, len(text), len(text)})
			break
		}
		end := start + nl
		next := end + 1
		if end > start && text[end-1] == '\r' {
			end--
		}
		out = append(out, [3]int{start, end, next})
		start = next
	}
	return out
}

// fence reports the backtick count of a fence line (0 if it is not one;
// up to three spaces of indentation are allowed) and the rest of the line.
func fence(line string) (int, string) {
	trimmed := strings.TrimLeft(line, " ")
	if len(line)-len(trimmed) > 3 {
		return 0, ""
	}
	n := 0
	for n < len(trimmed) && trimmed[n] == '`' {
		n++
	}
	if n < 3 || strings.Contains(trimmed[n:], "`") {
		return 0, ""
	}
	return n, trimmed[n:]
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
