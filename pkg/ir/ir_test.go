package ir

import (
	"strings"
	"testing"
)

const rows = `[{"id":1,"s":"a"},{"id":2,"s":"b"}]`

func userReq(texts ...string) *Request {
	m := Message{Role: "user"}
	for i, t := range texts {
		m.Blocks = append(m.Blocks, Block{Kind: Text, Loc: Locator{Message: 0, Part: i}, Text: t})
	}
	return &Request{Messages: []Message{m}}
}

func TestDetectData(t *testing.T) {
	for _, c := range []struct {
		name, text string
		want       []string // detected payloads, in order
	}{
		{"whole text", "  " + rows + "\n", []string{"  " + rows + "\n"}},
		{"prose", "how many refunded?", nil},
		{"scalar", "42", nil},
		{"fenced json", "Orders:\n```json\n" + rows + "\n```\nHow many?", []string{rows}},
		{"unlabelled fence", "```\n" + rows + "\n```", []string{rows}},
		{"crlf", "Orders:\r\n```json\r\n" + rows + "\r\n```\r\n", []string{rows}},
		{"two fences", "a\n```json\n" + rows + "\n```\nb\n````JSON\n{\"x\":[1]}\n````\n", []string{rows, `{"x":[1]}`}},
		{"other language", "```python\n" + rows + "\n```", nil},
		{"json inside other fence", "```md\n```json\n" + rows + "\n```\n```", nil},
		{"invalid json", "```json\n{\"a\":\n```", nil},
		{"unclosed", "```json\n" + rows, nil},
		{"short closing fence", "````json\n" + rows + "\n```\n", nil},
	} {
		req := userReq(c.text)
		req.DetectData()
		var got []string
		for _, b := range req.DataBlocks() {
			got = append(got, string(b.Data))
		}
		if strings.Join(got, "|") != strings.Join(c.want, "|") {
			t.Errorf("%s: got %q, want %q", c.name, got, c.want)
		}
	}
}

func TestDetectDataOnlyUserText(t *testing.T) {
	req := &Request{Messages: []Message{
		{Role: "assistant", Blocks: []Block{{Kind: Text, Text: rows}}},
		{Role: "user", Blocks: []Block{{Kind: Other}, {Kind: ToolResult, Text: "x"}}},
	}}
	req.DetectData()
	if n := len(req.DataBlocks()); n != 0 {
		t.Fatalf("%d data blocks", n)
	}
}

func TestRenderedText(t *testing.T) {
	text := "Orders:\n```json\n" + rows + "\n```\nLogs:\n```\n[{\"l\":1}]\n```\nHow many?"
	req := userReq(text, rows)
	req.DetectData()
	db := req.DataBlocks()
	if len(db) != 3 {
		t.Fatalf("%d data blocks", len(db))
	}
	loc := db[0].Loc
	if req.RenderedText(loc) != text {
		t.Fatal("untransformed text changed")
	}
	db[0].Transform = &AppliedTransform{Codec: "toon", Encoded: []byte("[2]{id,s}:\n  1,a\n  2,b")}
	db[1].Transform = &AppliedTransform{Codec: "toon", Encoded: []byte("[1]{l}:\n  1")}
	want := "Orders:\n```toon\n[2]{id,s}:\n  1,a\n  2,b\n```\nLogs:\n```\n[1]{l}:\n  1\n```\nHow many?"
	if got := req.RenderedText(loc); got != want {
		t.Fatalf("got\n%s\nwant\n%s", got, want)
	}
	db[2].Transform = &AppliedTransform{Codec: "toon", Encoded: []byte("whole")}
	if got := req.RenderedText(db[2].Loc); got != "whole" {
		t.Fatalf("whole-text block rendered %q", got)
	}
}
