package bench

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/rand/v2"
	"net/http"
	"strconv"
	"time"
)

const systemPrompt = `You are a data assistant inside an application. Answer the user's question using only the tool result in the conversation. Reply with only the final answer value: no words, units, currency symbols or explanation.`

// Call is one model request: a question whose data arrives as a tool result.
type Call struct {
	Model      string
	Primer     string
	Question   string
	Tool       string
	ToolArgs   string
	ToolResult string
	MaxTokens  int
}

// Result is what a provider returned.
type Result struct {
	Text         string
	InputTokens  int
	OutputTokens int
	Latency      time.Duration
}

// Client sends a Call to one provider API.
type Client interface {
	Do(ctx context.Context, c Call) (*Result, error)
}

func system(c Call) string {
	if c.Primer == "" {
		return systemPrompt
	}
	return systemPrompt + "\n\n" + c.Primer
}

// OpenAI speaks the OpenAI-compatible Chat Completions API.
type OpenAI struct {
	BaseURL, APIKey string
	HTTP            *http.Client
}

func (o *OpenAI) Do(ctx context.Context, c Call) (*Result, error) {
	body := map[string]any{
		"model":       c.Model,
		"max_tokens":  c.MaxTokens,
		"temperature": 0,
		"messages": []any{
			map[string]any{"role": "system", "content": system(c)},
			map[string]any{"role": "user", "content": c.Question},
			map[string]any{"role": "assistant", "content": "", "tool_calls": []any{map[string]any{
				"id": "call_1", "type": "function",
				"function": map[string]any{"name": c.Tool, "arguments": c.ToolArgs},
			}}},
			map[string]any{"role": "tool", "tool_call_id": "call_1", "content": c.ToolResult},
		},
		"tools": []any{map[string]any{"type": "function", "function": map[string]any{
			"name": c.Tool, "description": "Returns rows from the application database.",
			"parameters": map[string]any{"type": "object", "properties": map[string]any{}},
		}}},
		"tool_choice": "none",
	}
	var resp struct {
		Choices []struct {
			Message struct {
				Content          string `json:"content"`
				ReasoningContent string `json:"reasoning_content"`
			} `json:"message"`
		} `json:"choices"`
		Usage struct {
			PromptTokens     int `json:"prompt_tokens"`
			CompletionTokens int `json:"completion_tokens"`
		} `json:"usage"`
	}
	start := time.Now()
	if err := post(ctx, o.HTTP, o.BaseURL+"/chat/completions", map[string]string{"Authorization": "Bearer " + o.APIKey}, body, &resp); err != nil {
		return nil, err
	}
	if len(resp.Choices) == 0 {
		return nil, errors.New("openai: no choices")
	}
	return &Result{Text: resp.Choices[0].Message.Content, InputTokens: resp.Usage.PromptTokens, OutputTokens: resp.Usage.CompletionTokens, Latency: time.Since(start)}, nil
}

// Anthropic speaks the Anthropic Messages API.
type Anthropic struct {
	BaseURL, APIKey string
	HTTP            *http.Client
}

func (a *Anthropic) Do(ctx context.Context, c Call) (*Result, error) {
	var args any = map[string]any{}
	_ = json.Unmarshal([]byte(c.ToolArgs), &args)
	body := map[string]any{
		"model":       c.Model,
		"max_tokens":  c.MaxTokens,
		"temperature": 0,
		"system":      system(c),
		"messages": []any{
			map[string]any{"role": "user", "content": c.Question},
			map[string]any{"role": "assistant", "content": []any{map[string]any{
				"type": "tool_use", "id": "toolu_1", "name": c.Tool, "input": args,
			}}},
			map[string]any{"role": "user", "content": []any{map[string]any{
				"type": "tool_result", "tool_use_id": "toolu_1", "content": c.ToolResult,
			}}},
		},
		"tools": []any{map[string]any{
			"name": c.Tool, "description": "Returns rows from the application database.",
			"input_schema": map[string]any{"type": "object", "properties": map[string]any{}},
		}},
		"tool_choice": map[string]any{"type": "none"},
	}
	var resp struct {
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
		Usage struct {
			InputTokens  int `json:"input_tokens"`
			OutputTokens int `json:"output_tokens"`
		} `json:"usage"`
	}
	start := time.Now()
	hdr := map[string]string{"x-api-key": a.APIKey, "anthropic-version": "2023-06-01"}
	if err := post(ctx, a.HTTP, a.BaseURL+"/v1/messages", hdr, body, &resp); err != nil {
		return nil, err
	}
	var text string
	for _, b := range resp.Content {
		if b.Type == "text" {
			text += b.Text
		}
	}
	return &Result{Text: text, InputTokens: resp.Usage.InputTokens, OutputTokens: resp.Usage.OutputTokens, Latency: time.Since(start)}, nil
}

type statusError struct {
	code int
	body string
	wait time.Duration
}

func (e *statusError) Error() string { return fmt.Sprintf("HTTP %d: %s", e.code, e.body) }

// post sends JSON and decodes the reply, retrying 429/5xx with jittered
// exponential backoff (honouring Retry-After).
func post(ctx context.Context, hc *http.Client, url string, hdr map[string]string, body, out any) error {
	payload, err := json.Marshal(body)
	if err != nil {
		return err
	}
	backoff := 2 * time.Second
	for attempt := 0; ; attempt++ {
		err := once(ctx, hc, url, hdr, payload, out)
		var se *statusError
		if err == nil || attempt >= 6 || ctx.Err() != nil {
			return err
		}
		if errors.As(err, &se) && se.code != 429 && se.code < 500 {
			return err
		}
		wait := backoff + time.Duration(rand.Int64N(int64(backoff)))
		if se != nil && se.wait > wait {
			wait = se.wait
		}
		select {
		case <-time.After(wait):
		case <-ctx.Done():
			return ctx.Err()
		}
		backoff = min(backoff*2, 60*time.Second)
	}
}

func once(ctx context.Context, hc *http.Client, url string, hdr map[string]string, payload []byte, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	for k, v := range hdr {
		req.Header.Set(k, v)
	}
	resp, err := hc.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return err
	}
	if resp.StatusCode != http.StatusOK {
		se := &statusError{code: resp.StatusCode, body: string(data[:min(len(data), 300)])}
		if s, err := strconv.Atoi(resp.Header.Get("Retry-After")); err == nil {
			se.wait = time.Duration(s) * time.Second
		}
		return se
	}
	return json.Unmarshal(data, out)
}
