package markdown

import (
	"image"
	"testing"

	"dyamark/fonts"
	"dyamark/seq"

	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/text"
	"gioui.org/unit"
	"gioui.org/widget/material"
)

const testMarkdown = `# Document Title

Introduction paragraph with some text.

- Item 1
- Item 2

> A wise quote.

---

` + "```mermaid" + `
sequenceDiagram
Alice->>Bob: Hello
Bob-->>Alice: Hi
` + "```" + `

` + "```mermaid" + `
flowchart TD
A --> B
` + "```" + `

` + "```python" + `
print("Hello world")
` + "```"

func TestParseMarkdown(t *testing.T) {
	cache := NewDiagramCache()
	doc := Parse([]byte(testMarkdown), cache)

	if len(doc.Blocks) == 0 {
		t.Fatal("expected parsed blocks, got none")
	}

	// Check headings
	var h1Count, pCount, listCount, quoteCount, divCount, seqCount, mermaidCount, codeCount int
	for _, b := range doc.Blocks {
		switch blk := b.(type) {
		case *HeadingBlock:
			if blk.Level == 1 && blk.Text == "Document Title" {
				h1Count++
			}
		case *ParagraphBlock:
			pCount++
		case *ListBlock:
			listCount++
		case *QuoteBlock:
			quoteCount++
		case *DividerBlock:
			divCount++
		case *SequenceBlock:
			seqCount++
		case *MermaidOtherBlock:
			mermaidCount++
		case *CodeBlock:
			codeCount++
		}
	}

	if h1Count != 1 {
		t.Errorf("expected 1 H1 heading, got %d", h1Count)
	}
	if pCount < 1 {
		t.Errorf("expected at least 1 paragraph, got %d", pCount)
	}
	if listCount != 1 {
		t.Errorf("expected 1 list, got %d", listCount)
	}
	if quoteCount != 1 {
		t.Errorf("expected 1 quote, got %d", quoteCount)
	}
	if divCount != 1 {
		t.Errorf("expected 1 divider, got %d", divCount)
	}
	if seqCount != 1 {
		t.Errorf("expected 1 sequence diagram, got %d", seqCount)
	}
	if mermaidCount != 1 {
		t.Errorf("expected 1 other mermaid diagram, got %d", mermaidCount)
	}
	if codeCount != 1 {
		t.Errorf("expected 1 code block, got %d", codeCount)
	}
}

func TestDiagramCacheAndErrorHandling(t *testing.T) {
	cache := NewDiagramCache()

	validSource := `sequenceDiagram
Alice->>Bob: Valid message`
	ast1, diag1, err1, lastGood1 := cache.GetOrLayout(validSource, 0)
	if err1 != nil {
		t.Fatalf("unexpected error for valid diagram: %v", err1)
	}
	if ast1 == nil || diag1 == nil || lastGood1 == nil {
		t.Fatal("expected populated diagram")
	}

	// Re-retrieval from cache should return the exact same pointer
	ast2, diag2, err2, _ := cache.GetOrLayout(validSource, 0)
	if err2 != nil || ast1 != ast2 || diag1 != diag2 {
		t.Error("expected cached instance to be returned")
	}

	// Now introduce an error (unclosed loop)
	invalidSource := `sequenceDiagram
loop Unclosed
Alice->>Bob: Hi`
	_, _, errInvalid, lastGoodErr := cache.GetOrLayout(invalidSource, 0)
	if errInvalid == nil {
		t.Error("expected syntax error")
	}
	// Last good diagram should still be preserved
	if lastGoodErr != diag1 {
		t.Errorf("expected lastGood diagram preserved across errors")
	}
}

func TestLineIndex(t *testing.T) {
	source := []byte("line 1\nline 2\nline 3\n")
	idx := NewLineIndex(source)

	if l := idx.LineNumber(0); l != 1 {
		t.Errorf("offset 0 should be line 1, got %d", l)
	}
	if l := idx.LineNumber(7); l != 2 {
		t.Errorf("offset 7 should be line 2, got %d", l)
	}
	if l := idx.LineNumber(14); l != 3 {
		t.Errorf("offset 14 should be line 3, got %d", l)
	}
}

func TestViewLayout(t *testing.T) {
	cache := NewDiagramCache()
	doc := Parse([]byte(testMarkdown), cache)
	view := NewView(doc)

	collection, err := fonts.Collection()
	if err != nil {
		t.Fatal(err)
	}
	th := material.NewTheme()
	th.Shaper = text.NewShaper(text.WithCollection(collection), text.NoSystemFonts())
	mTheme := &Theme{
		Background:        seq.HexColor("#0b0f19", 255),
		Foreground:        seq.HexColor("#e6edf3", 255),
		Accent:            seq.HexColor("#ffd60a", 255),
		LighterBackground: seq.HexColor("#1f242c", 255),
		Seq:               seq.DefaultTheme(),
	}

	ops := new(op.Ops)
	gtx := layout.Context{
		Ops: ops,
		Metric: unit.Metric{
			PxPerDp: 1.0,
			PxPerSp: 1.0,
		},
		Constraints: layout.Exact(image.Pt(1000, 800)),
	}

	dims := view.Layout(gtx, th, mTheme)
	if dims.Size.X <= 0 || dims.Size.Y <= 0 {
		t.Errorf("invalid view dimensions: %v", dims.Size)
	}

	// Focus navigation
	view.FocusNext()
	if view.FocusedIndex != 0 {
		t.Errorf("expected focused index 0, got %d", view.FocusedIndex)
	}
}
