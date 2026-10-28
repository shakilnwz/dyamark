package markdown

import "testing"

// The word count is of what a reader sees: markup and diagrams don't
// count, code does, and markup inside a word doesn't split it.
func TestWords(t *testing.T) {
	for _, tc := range []struct {
		src  string
		want int
	}{
		{"", 0},
		{"# Hello world\n\nOne two three.\n", 5},
		{"foo**bar** and *baz*\n", 3},
		{"line one\nline two\n", 4},
		{"- one\n- two three\n\n> four\n", 4},
		{"| a | b |\n|---|---|\n| c d | e |\n", 5},
		{"see [the docs](https://example.com) or <https://x.org>\n", 5},
		{"```go\nfmt.Println(x)\nreturn\n```\n", 2},
		{"text\n\n```mermaid\nsequenceDiagram\nA->>B: hello there\n```\n", 1},
		{"```mermaid\npie\n\"a\" : 1\n```\n", 0},
		{"<div>html here</div>\n\nword\n", 1},
		{"sequenceDiagram\nA->>B: hi\n", 0},
	} {
		if got := Parse([]byte(tc.src), nil).Words; got != tc.want {
			t.Errorf("%q: %d words, want %d", tc.src, got, tc.want)
		}
	}
}
