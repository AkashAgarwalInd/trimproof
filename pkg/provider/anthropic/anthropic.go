// Package anthropic adapts the Anthropic Messages API to the IR.
package anthropic

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/AkashAgarwalInd/context-mesh/pkg/ir"
	"github.com/AkashAgarwalInd/context-mesh/pkg/provider"
)

// Adapter implements provider.Adapter for /v1/messages.
type Adapter struct{}

func (Adapter) Name() string { return "anthropic" }

type wireRequest struct {
	Model        string                            `json:"model"`
	Stream       bool                              `json:"stream"`
	System       json.RawMessage                   `json:"system"`
	Messages     []wireMessage                     `json:"messages"`
	Tools        []wireTool                        `json:"tools"`
	OutputFormat json.RawMessage                   `json:"output_format"`
	OutputConfig *struct{ Format json.RawMessage } `json:"output_config"`
}

type wireMessage struct {
	Role    string          `json:"role"`
	Content json.RawMessage `json:"content"`
}

type wireTool struct {
	Name        string          `json:"name"`
	InputSchema json.RawMessage `json:"input_schema"`
}

type wireBlock struct {
	Type      string          `json:"type"`
	Text      string          `json:"text"`
	ID        string          `json:"id"`
	Name      string          `json:"name"`
	Input     json.RawMessage `json:"input"`
	ToolUseID string          `json:"tool_use_id"`
	Content   json.RawMessage `json:"content"`
	IsError   bool            `json:"is_error"`
}

func (Adapter) ParseRequest(wire []byte) (*ir.Request, error) {
	var w wireRequest
	if err := json.Unmarshal(wire, &w); err != nil {
		return nil, fmt.Errorf("anthropic: %w", err)
	}
	req := &ir.Request{Provider: "anthropic", Model: w.Model, Stream: w.Stream, Raw: wire}
	req.StructuredOutput = len(w.OutputFormat) > 0 && string(w.OutputFormat) != "null" ||
		w.OutputConfig != nil && len(w.OutputConfig.Format) > 0 && string(w.OutputConfig.Format) != "null"
	for _, t := range w.Tools {
		req.Tools = append(req.Tools, ir.ToolDef{Name: t.Name, Schema: t.InputSchema})
	}
	sys, err := parseContent(w.System, -1, nil)
	if err != nil {
		return nil, fmt.Errorf("anthropic: system: %w", err)
	}
	req.System = sys
	toolNames := map[string]string{} // tool_use id -> name
	for i, m := range w.Messages {
		blocks, err := parseContent(m.Content, i, toolNames)
		if err != nil {
			return nil, fmt.Errorf("anthropic: messages[%d]: %w", i, err)
		}
		req.Messages = append(req.Messages, ir.Message{Role: m.Role, Blocks: blocks})
	}
	return req, nil
}

func parseContent(raw json.RawMessage, msg int, toolNames map[string]string) ([]ir.Block, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return nil, nil
	}
	if raw[0] == '"' {
		var s string
		if err := json.Unmarshal(raw, &s); err != nil {
			return nil, err
		}
		return []ir.Block{{Kind: ir.Text, Loc: ir.Locator{Message: msg, Part: -1}, Text: s}}, nil
	}
	var parts []wireBlock
	if err := json.Unmarshal(raw, &parts); err != nil {
		return nil, err
	}
	var out []ir.Block
	for j, p := range parts {
		loc := ir.Locator{Message: msg, Part: j}
		switch p.Type {
		case "text":
			out = append(out, ir.Block{Kind: ir.Text, Loc: loc, Text: p.Text})
		case "tool_use":
			if toolNames != nil {
				toolNames[p.ID] = p.Name
			}
			out = append(out, ir.Block{Kind: ir.ToolUse, Loc: loc, ToolName: p.Name, ToolUseID: p.ID, Data: p.Input})
		case "tool_result":
			b := ir.Block{Kind: ir.ToolResult, Loc: loc, ToolUseID: p.ToolUseID, IsError: p.IsError, ToolName: toolNames[p.ToolUseID]}
			if text, ok := singleText(p.Content); ok {
				b.Text = text
				if provider.LooksLikeJSONData(text) {
					b.Data = []byte(text)
				}
			}
			out = append(out, b)
		default:
			out = append(out, ir.Block{Kind: ir.Other, Loc: loc})
		}
	}
	return out, nil
}

// singleText extracts the text of a tool_result whose content is a string
// or an array holding exactly one text block. Anything else (images,
// several blocks) is left alone.
func singleText(raw json.RawMessage) (string, bool) {
	if len(raw) == 0 {
		return "", false
	}
	if raw[0] == '"' {
		var s string
		return s, json.Unmarshal(raw, &s) == nil
	}
	var parts []wireBlock
	if json.Unmarshal(raw, &parts) != nil || len(parts) != 1 || parts[0].Type != "text" {
		return "", false
	}
	return parts[0].Text, true
}

// RenderRequest patches transformed tool results and the primer into the
// original body.
func (Adapter) RenderRequest(req *ir.Request) ([]byte, error) {
	if !req.Transformed() && req.Primer == "" {
		return req.Raw, nil
	}
	var top map[string]json.RawMessage
	if err := json.Unmarshal(req.Raw, &top); err != nil {
		return nil, err
	}
	var msgs []json.RawMessage
	if err := json.Unmarshal(top["messages"], &msgs); err != nil {
		return nil, err
	}
	for _, b := range req.DataBlocks() {
		if b.Transform == nil {
			continue
		}
		patched, err := patchBlock(msgs[b.Loc.Message], b)
		if err != nil {
			return nil, err
		}
		msgs[b.Loc.Message] = patched
	}
	var err error
	if top["messages"], err = provider.Marshal(msgs); err != nil {
		return nil, err
	}
	if req.Primer != "" {
		if top["system"], err = appendSystem(top["system"], req.Primer); err != nil {
			return nil, err
		}
	}
	return provider.Marshal(top)
}

func patchBlock(rawMsg json.RawMessage, b *ir.Block) (json.RawMessage, error) {
	var msg map[string]json.RawMessage
	if err := json.Unmarshal(rawMsg, &msg); err != nil {
		return nil, err
	}
	text := string(b.Transform.Encoded)
	if b.Loc.Part < 0 {
		msg["content"] = provider.RawString(text)
		return provider.Marshal(msg)
	}
	var parts []map[string]json.RawMessage
	if err := json.Unmarshal(msg["content"], &parts); err != nil {
		return nil, err
	}
	if b.Loc.Part >= len(parts) {
		return nil, errors.New("anthropic: locator out of range")
	}
	part := parts[b.Loc.Part]
	switch b.Kind {
	case ir.ToolResult:
		// Collapse string or single-text-block content to the encoded string.
		part["content"] = provider.RawString(text)
	case ir.MarkedData:
		part["text"] = provider.RawString(text)
	}
	var err error
	if msg["content"], err = provider.Marshal(parts); err != nil {
		return nil, err
	}
	return provider.Marshal(msg)
}

// appendSystem adds the primer after any existing system content, keeping
// existing blocks (and their cache_control) intact.
func appendSystem(sys json.RawMessage, primer string) (json.RawMessage, error) {
	if len(sys) == 0 || string(sys) == "null" {
		return provider.RawString(primer), nil
	}
	if sys[0] == '"' {
		var s string
		if err := json.Unmarshal(sys, &s); err != nil {
			return nil, err
		}
		return provider.RawString(s + "\n\n" + primer), nil
	}
	var parts []json.RawMessage
	if err := json.Unmarshal(sys, &parts); err != nil {
		return nil, err
	}
	block, _ := provider.Marshal(map[string]string{"type": "text", "text": primer})
	return provider.Marshal(append(parts, block))
}

type wireResponse struct {
	Content    []wireBlock `json:"content"`
	StopReason string      `json:"stop_reason"`
	Usage      struct {
		InputTokens              int `json:"input_tokens"`
		OutputTokens             int `json:"output_tokens"`
		CacheReadInputTokens     int `json:"cache_read_input_tokens"`
		CacheCreationInputTokens int `json:"cache_creation_input_tokens"`
	} `json:"usage"`
}

func (Adapter) ParseResponse(req *ir.Request, wire []byte) (*ir.Response, error) {
	var w wireResponse
	if err := json.Unmarshal(wire, &w); err != nil {
		return nil, fmt.Errorf("anthropic: response: %w", err)
	}
	resp := &ir.Response{StopReason: w.StopReason, Raw: wire, Usage: ir.Usage{
		// input_tokens excludes cache reads/writes; report the total prompt.
		InputTokens:       w.Usage.InputTokens + w.Usage.CacheReadInputTokens + w.Usage.CacheCreationInputTokens,
		OutputTokens:      w.Usage.OutputTokens,
		CachedInputTokens: w.Usage.CacheReadInputTokens,
	}}
	for _, b := range w.Content {
		switch b.Type {
		case "text":
			resp.Text += b.Text
		case "tool_use":
			args := b.Input
			if len(args) == 0 {
				args = json.RawMessage("{}")
			}
			resp.ToolCalls = append(resp.ToolCalls, ir.ToolCall{ID: b.ID, Name: b.Name, Args: args})
		}
	}
	if req != nil && req.StructuredOutput && resp.Text != "" {
		resp.Structured = json.RawMessage(resp.Text)
	}
	return resp, nil
}

// AssembleStream implements provider.StreamAssembler.
func (Adapter) AssembleStream(req *ir.Request, sse []byte) (*ir.Response, error) {
	return provider.AssembleAnthropic(req, sse), nil
}
