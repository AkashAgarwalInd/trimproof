package provider

import (
	"bufio"
	"bytes"
	"encoding/json"
	"sort"

	"github.com/AkashAgarwalInd/trimproof/pkg/ir"
)

// StreamAssembler rebuilds a complete response from a buffered SSE stream,
// so routes with Tier 1 validation can validate before releasing.
type StreamAssembler interface {
	AssembleStream(req *ir.Request, sse []byte) (*ir.Response, error)
}

// SSEData yields the payload of each "data:" line.
func SSEData(sse []byte, f func(data []byte)) {
	sc := bufio.NewScanner(bytes.NewReader(sse))
	sc.Buffer(make([]byte, 64<<10), 16<<20)
	for sc.Scan() {
		line := sc.Bytes()
		if d, ok := bytes.CutPrefix(line, []byte("data:")); ok {
			f(bytes.TrimSpace(d))
		}
	}
}

// AssembleAnthropic rebuilds a Messages API response from its event stream.
func AssembleAnthropic(req *ir.Request, sse []byte) *ir.Response {
	type block struct {
		typ, id, name string
		text, input   bytes.Buffer
	}
	blocks := map[int]*block{}
	resp := &ir.Response{Raw: sse}
	SSEData(sse, func(d []byte) {
		var ev struct {
			Type    string `json:"type"`
			Index   int    `json:"index"`
			Message struct {
				Usage struct {
					InputTokens              int `json:"input_tokens"`
					CacheReadInputTokens     int `json:"cache_read_input_tokens"`
					CacheCreationInputTokens int `json:"cache_creation_input_tokens"`
				} `json:"usage"`
			} `json:"message"`
			ContentBlock struct {
				Type string `json:"type"`
				ID   string `json:"id"`
				Name string `json:"name"`
			} `json:"content_block"`
			Delta struct {
				Type        string `json:"type"`
				Text        string `json:"text"`
				PartialJSON string `json:"partial_json"`
				StopReason  string `json:"stop_reason"`
			} `json:"delta"`
			Usage struct {
				OutputTokens int `json:"output_tokens"`
			} `json:"usage"`
		}
		if json.Unmarshal(d, &ev) != nil {
			return
		}
		switch ev.Type {
		case "message_start":
			u := ev.Message.Usage
			resp.Usage.InputTokens = u.InputTokens + u.CacheReadInputTokens + u.CacheCreationInputTokens
			resp.Usage.CachedInputTokens = u.CacheReadInputTokens
		case "content_block_start":
			blocks[ev.Index] = &block{typ: ev.ContentBlock.Type, id: ev.ContentBlock.ID, name: ev.ContentBlock.Name}
		case "content_block_delta":
			b := blocks[ev.Index]
			if b == nil {
				return
			}
			b.text.WriteString(ev.Delta.Text)
			b.input.WriteString(ev.Delta.PartialJSON)
		case "message_delta":
			if ev.Delta.StopReason != "" {
				resp.StopReason = ev.Delta.StopReason
			}
			if ev.Usage.OutputTokens > 0 {
				resp.Usage.OutputTokens = ev.Usage.OutputTokens
			}
		}
	})
	idx := make([]int, 0, len(blocks))
	for i := range blocks {
		idx = append(idx, i)
	}
	sort.Ints(idx)
	for _, i := range idx {
		b := blocks[i]
		switch b.typ {
		case "text":
			resp.Text += b.text.String()
		case "tool_use":
			args := b.input.Bytes()
			if len(args) == 0 {
				args = []byte("{}")
			}
			resp.ToolCalls = append(resp.ToolCalls, ir.ToolCall{ID: b.id, Name: b.name, Args: append(json.RawMessage(nil), args...)})
		}
	}
	if req != nil && req.StructuredOutput && resp.Text != "" {
		resp.Structured = json.RawMessage(resp.Text)
	}
	return resp
}

// AssembleOpenAI rebuilds a Chat Completions response from its chunks.
func AssembleOpenAI(req *ir.Request, sse []byte) *ir.Response {
	type call struct {
		id, name string
		args     bytes.Buffer
	}
	calls := map[int]*call{}
	var text bytes.Buffer
	resp := &ir.Response{Raw: sse}
	SSEData(sse, func(d []byte) {
		if string(d) == "[DONE]" {
			return
		}
		var ch struct {
			Choices []struct {
				Delta struct {
					Content   string `json:"content"`
					ToolCalls []struct {
						Index    int    `json:"index"`
						ID       string `json:"id"`
						Function struct {
							Name      string `json:"name"`
							Arguments string `json:"arguments"`
						} `json:"function"`
					} `json:"tool_calls"`
				} `json:"delta"`
				FinishReason string `json:"finish_reason"`
			} `json:"choices"`
			Usage *struct {
				PromptTokens     int `json:"prompt_tokens"`
				CompletionTokens int `json:"completion_tokens"`
			} `json:"usage"`
		}
		if json.Unmarshal(d, &ch) != nil {
			return
		}
		if ch.Usage != nil {
			resp.Usage.InputTokens, resp.Usage.OutputTokens = ch.Usage.PromptTokens, ch.Usage.CompletionTokens
		}
		if len(ch.Choices) == 0 {
			return
		}
		c := ch.Choices[0]
		text.WriteString(c.Delta.Content)
		for _, tc := range c.Delta.ToolCalls {
			cl := calls[tc.Index]
			if cl == nil {
				cl = &call{}
				calls[tc.Index] = cl
			}
			if tc.ID != "" {
				cl.id = tc.ID
			}
			if tc.Function.Name != "" {
				cl.name = tc.Function.Name
			}
			cl.args.WriteString(tc.Function.Arguments)
		}
		if c.FinishReason != "" {
			resp.StopReason = c.FinishReason
		}
	})
	resp.Text = text.String()
	idx := make([]int, 0, len(calls))
	for i := range calls {
		idx = append(idx, i)
	}
	sort.Ints(idx)
	for _, i := range idx {
		c := calls[i]
		args := c.args.Bytes()
		if len(args) == 0 {
			args = []byte("{}")
		}
		resp.ToolCalls = append(resp.ToolCalls, ir.ToolCall{ID: c.id, Name: c.name, Args: append(json.RawMessage(nil), args...)})
	}
	if req != nil && req.StructuredOutput && resp.Text != "" {
		resp.Structured = json.RawMessage(resp.Text)
	}
	return resp
}
