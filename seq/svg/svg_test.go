package svg

import (
	"strings"
	"testing"

	"dyamark/seq/layout"
	"dyamark/seq/parse"
)

func TestSvgRender(t *testing.T) {
	src := `sequenceDiagram
Alice->>Bob: Hello
`
	d, err := parse.Parse(src)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	diag, err := layout.Layout(d, layout.Default())
	if err != nil {
		t.Fatalf("layout: %v", err)
	}
	svgStr, err := Render(diag)
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	if !strings.HasPrefix(svgStr, "<svg") {
		t.Fatalf("expected <svg prefix, got %s", svgStr[:30])
	}
	if !strings.Contains(svgStr, "Hello") {
		t.Fatalf("expected Hello in svg")
	}
}
