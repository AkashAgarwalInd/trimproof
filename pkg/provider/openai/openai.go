// Package openai adapts the OpenAI-compatible Chat Completions API to the IR.
package openai

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/AkashAgarwalInd/trimproof/pkg/ir"
	"github.com/AkashAgarwalInd/trimproof/pkg/provider"
)

// Adapter implements provider.Adapter for /v1/chat/completions.
type Adapter struct{}

func (Adapter) Name() string { return "openai" }

type wireRequest struct {
	Model          string        `json:"model"`
	Stream         bool          `json:"stream"`
	Messages       []wireMessage `json:"messages"`
	Tools          []wireTool    `json:"tools"`
	ResponseFormat *struct {
		Type string `json:"type"`
	} `json:"response_format"`
}

type wireMessage struct {
	Role       string          `json:"role"`
	Content    json.RawMessage `json:"content"`
	ToolCallID string          `json:"tool_call_id"`
	ToolCalls  []wireToolCall  `json:"tool_calls"`
}

type wireToolCall struct {
	ID       string `json:"id"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

type wireTool struct {
	Function struct {
		Name       string          `json:"name"`
		Parameters json.RawMessage `json:"parameters"`
	} `json:"function"`
}

type wirePart struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

func (Adapter) ParseRequest(wire []byte) (*ir.Request, error) {
	var w wireRequest
	if err := json.Unmarshal(wire, &w); err != nil {
		return nil, fmt.Errorf("openai: %w", err)
	}
	req := &ir.Request{Provider: "openai", Model: w.Model, Stream: w.Stream, Raw: wire}
	if w.ResponseFormat != nil && (w.ResponseFormat.Type == "json_schema" || w.ResponseFormat.Type == "json_object") {
		req.StructuredOutput = true
	}
	for _, t := range w.Tools {
		req.Tools = append(req.Tools, ir.ToolDef{Name: t.Function.Name, Schema: t.Function.Parameters})
	}
	toolNames := map[string]string{}
	for i, m := range w.Messages {
		blocks, err := parseContent(m.Content, i)
		if err != nil {
			return nil, fmt.Errorf("openai: messages[%d]: %w", i, err)
		}
		switch m.Role {
		case "system", "developer":
			req.System = append(req.System, blocks...)
		case "assistant":
			for _, tc := range m.ToolCalls {
				toolNames[tc.ID] = tc.Function.Name
				blocks = append(blocks, ir.Block{Kind: ir.ToolUse, Loc: ir.Locator{Message: i, Part: -2},
					ToolName: tc.Function.Name, ToolUseID: tc.ID, Data: []byte(tc.Function.Arguments)})
			}
		case "tool":
			// The whole tool message is one result when it is a single text.
			if len(blocks) == 1 && blocks[0].Kind == ir.Text {
				b := &blocks[0]
				b.Kind, b.ToolUseID, b.ToolName = ir.ToolResult, m.ToolCallID, toolNames[m.ToolCallID]
				if provider.LooksLikeJSONData(b.Text) {
					b.Data = []byte(b.Text)
				}
			}
		}
		req.Messages = append(req.Messages, ir.Message{Role: m.Role, Blocks: blocks})
	}
	return req, nil
}

func parseContent(raw json.RawMessage, msg int) ([]ir.Block, error) {
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
	var parts []wirePart
	if err := json.Unmarshal(raw, &parts); err != nil {
		return nil, err
	}
	out := make([]ir.Block, len(parts))
	for j, p := range parts {
		loc := ir.Locator{Message: msg, Part: j}
		if p.Type == "text" {
			out[j] = ir.Block{Kind: ir.Text, Loc: loc, Text: p.Text}
		} else {
			out[j] = ir.Block{Kind: ir.Other, Loc: loc}
		}
	}
	return out, nil
}

// RenderRequest patches transformed blocks into the original body and adds
// the primer to the first system/developer message (or a new one).
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
		var m map[string]json.RawMessage
		if err := json.Unmarshal(msgs[b.Loc.Message], &m); err != nil {
			return nil, err
		}
		text := provider.RawString(req.RenderedText(b.Loc))
		if b.Loc.Part < 0 {
			m["content"] = text
		} else {
			var parts []map[string]json.RawMessage
			if err := json.Unmarshal(m["content"], &parts); err != nil {
				return nil, err
			}
			if b.Loc.Part >= len(parts) {
				return nil, errors.New("openai: locator out of range")
			}
			parts[b.Loc.Part]["text"] = text
			var err error
			if m["content"], err = provider.Marshal(parts); err != nil {
				return nil, err
			}
		}
		var err error
		if msgs[b.Loc.Message], err = provider.Marshal(m); err != nil {
			return nil, err
		}
	}
	if req.Primer != "" {
		var err error
		if msgs, err = addPrimer(msgs, req.Primer); err != nil {
			return nil, err
		}
	}
	var err error
	if top["messages"], err = provider.Marshal(msgs); err != nil {
		return nil, err
	}
	return provider.Marshal(top)
}

func addPrimer(msgs []json.RawMessage, primer string) ([]json.RawMessage, error) {
	for i, raw := range msgs {
		var m map[string]json.RawMessage
		if err := json.Unmarshal(raw, &m); err != nil {
			return nil, err
		}
		var role string
		_ = json.Unmarshal(m["role"], &role)
		if role != "system" && role != "developer" {
			continue
		}
		c := m["content"]
		if len(c) > 0 && c[0] == '"' {
			var s string
			if err := json.Unmarshal(c, &s); err != nil {
				return nil, err
			}
			m["content"] = provider.RawString(s + "\n\n" + primer)
		} else {
			var parts []json.RawMessage
			if err := json.Unmarshal(c, &parts); err != nil {
				return nil, err
			}
			p, _ := provider.Marshal(map[string]string{"type": "text", "text": primer})
			var err error
			if m["content"], err = provider.Marshal(append(parts, p)); err != nil {
				return nil, err
			}
		}
		var err error
		msgs[i], err = provider.Marshal(m)
		return msgs, err
	}
	sys, _ := provider.Marshal(map[string]string{"role": "system", "content": primer})
	return append([]json.RawMessage{sys}, msgs...), nil
}

type wireResponse struct {
	Choices []struct {
		FinishReason string `json:"finish_reason"`
		Message      struct {
			Content   *string        `json:"content"`
			ToolCalls []wireToolCall `json:"tool_calls"`
		} `json:"message"`
	} `json:"choices"`
	Usage struct {
		PromptTokens        int `json:"prompt_tokens"`
		CompletionTokens    int `json:"completion_tokens"`
		PromptTokensDetails *struct {
			CachedTokens int `json:"cached_tokens"`
		} `json:"prompt_tokens_details"`
	} `json:"usage"`
}

func (Adapter) ParseResponse(req *ir.Request, wire []byte) (*ir.Response, error) {
	var w wireResponse
	if err := json.Unmarshal(wire, &w); err != nil {
		return nil, fmt.Errorf("openai: response: %w", err)
	}
	resp := &ir.Response{Raw: wire, Usage: ir.Usage{InputTokens: w.Usage.PromptTokens, OutputTokens: w.Usage.CompletionTokens}}
	if d := w.Usage.PromptTokensDetails; d != nil {
		resp.Usage.CachedInputTokens = d.CachedTokens
	}
	if len(w.Choices) == 0 {
		return resp, nil
	}
	c := w.Choices[0]
	resp.StopReason = c.FinishReason
	if c.Message.Content != nil {
		resp.Text = *c.Message.Content
	}
	for _, tc := range c.Message.ToolCalls {
		args := json.RawMessage(tc.Function.Arguments)
		if len(args) == 0 {
			args = json.RawMessage("{}")
		}
		resp.ToolCalls = append(resp.ToolCalls, ir.ToolCall{ID: tc.ID, Name: tc.Function.Name, Args: args})
	}
	if req != nil && req.StructuredOutput && resp.Text != "" {
		resp.Structured = json.RawMessage(resp.Text)
	}
	return resp, nil
}

// AssembleStream implements provider.StreamAssembler.
func (Adapter) AssembleStream(req *ir.Request, sse []byte) (*ir.Response, error) {
	return provider.AssembleOpenAI(req, sse), nil
}
