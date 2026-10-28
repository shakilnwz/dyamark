package markdown

import (
	"reflect"
	"testing"
)

const fence = "```"

// A ```mermaid block is a sequence diagram only when its first statement,
// after front matter, directives and comments, is sequenceDiagram.
func TestMermaidTypeDetection(t *testing.T) {
	tests := []struct {
		name     string
		code     string
		sequence bool
		diagType string
	}{
		{"plain sequence", "sequenceDiagram\nA->>B: hi\n", true, ""},
		{"front matter sequence", "---\ntitle: Login\n---\nsequenceDiagram\nA->>B: hi\n", true, ""},
		{"init sequence", "%%{init: {'theme': 'dark'}}%%\nsequenceDiagram\nA->>B: hi\n", true, ""},
		{"comment sequence", "%% the login flow\n\nsequenceDiagram\nA->>B: hi\n", true, ""},
		{"multi-line init sequence", "%%{init: {\n  'theme': 'dark'\n}}%%\nsequenceDiagram\nA->>B: hi\n", true, ""},
		{"front matter flowchart", "---\ntitle: Flow\n---\nflowchart TD\nA-->B\n", false, "flowchart"},
		{"init flowchart", "%%{init: {'theme': 'dark'}}%%\nflowchart TD\nA-->B\n", false, "flowchart"},
		{"front matter and init class", "---\nconfig:\n  look: handDrawn\n---\n%%{init: {}}%%\n%% c\nclassDiagram\nclass A\n", false, "classDiagram"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			doc := Parse([]byte(fence+"mermaid\n"+tt.code+fence+"\n"), nil)
			if len(doc.Blocks) != 1 {
				t.Fatalf("got %d blocks, want 1", len(doc.Blocks))
			}
			switch b := doc.Blocks[0].(type) {
			case *SequenceBlock:
				if !tt.sequence {
					t.Fatalf("drawn as a sequence diagram, want %s with the mmdc hint", tt.diagType)
				}
				if b.Error != nil {
					t.Errorf("sequence diagram failed: %v", b.Error)
				}
			case *MermaidOtherBlock:
				if tt.sequence {
					t.Fatalf("got %s with the mmdc hint, want a sequence diagram", b.DiagramType)
				}
				if b.DiagramType != tt.diagType {
					t.Errorf("diagram type %q, want %q", b.DiagramType, tt.diagType)
				}
			default:
				t.Fatalf("got %T", b)
			}
		})
	}
}

// A raw .mmd file is wrapped in a fence when it's a sequence diagram, with
// or without front matter in front.
func TestRawSequenceSource(t *testing.T) {
	for _, src := range []string{
		"sequenceDiagram\nA->>B: hi\n",
		"---\ntitle: Login\n---\nsequenceDiagram\nA->>B: hi\n",
		"%%{init: {'theme': 'forest'}}%%\nsequenceDiagram\nA->>B: hi\n",
	} {
		doc := Parse([]byte(src), nil)
		if len(doc.Blocks) != 1 || len(doc.Sequences) != 1 {
			t.Errorf("%q: got %d blocks and %d sequence diagrams, want one diagram", src, len(doc.Blocks), len(doc.Sequences))
			continue
		}
		if err := doc.Sequences[0].Error; err != nil {
			t.Errorf("%q: %v", src, err)
		}
	}

	// Its diagram starts on the file's first line, after blank lines.
	for src, want := range map[string]int{
		"sequenceDiagram\nA->>B: hi\n":     1,
		"\n\nsequenceDiagram\nA->>B: hi\n": 3,
	} {
		if got := Parse([]byte(src), nil).Sequences[0].StartLine; got != want {
			t.Errorf("%q starts on line %d, want %d", src, got, want)
		}
	}

	// A Markdown file with YAML front matter stays Markdown.
	doc := Parse([]byte("---\ntitle: Notes\n---\n\n# Notes\n"), nil)
	if len(doc.Sequences) != 0 {
		t.Error("Markdown with front matter was read as a diagram")
	}
}

// Escapes and entity references are resolved; code spans are kept as
// written.
func TestInlineText(t *testing.T) {
	tests := []struct{ src, want string }{
		{`AT&amp;T`, "AT&T"},
		{`\*x\*`, "*x*"},
		{`&copy; &#169; &#xA9;`, "© © ©"},
		{`\&amp;`, "&amp;"},
		{"`a&amp;b \\*`", `a&amp;b \*`},
		{"one  \ntwo\nthree", "one\ntwo three"},
		{"<https://example.com/?a=1&b=2>", "https://example.com/?a=1&b=2"},
	}
	for _, tt := range tests {
		doc := Parse([]byte(tt.src), nil)
		if len(doc.Blocks) != 1 {
			t.Errorf("%q: got %d blocks", tt.src, len(doc.Blocks))
			continue
		}
		if got := doc.Blocks[0].(*ParagraphBlock).Text; got != tt.want {
			t.Errorf("%q: got %q, want %q", tt.src, got, tt.want)
		}
	}

	doc := Parse([]byte("# Q&amp;A\n\n| a \\| b | x&lt;y |\n|---|---|\n| 1 | 2 |\n"), nil)
	if got := doc.Blocks[0].(*HeadingBlock).Text; got != "Q&A" {
		t.Errorf("heading: got %q", got)
	}
	if got := doc.Blocks[1].(*TableBlock).Headers; !reflect.DeepEqual(got, []string{"a | b", "x<y"}) {
		t.Errorf("table headers: got %q", got)
	}
}

func TestListStructure(t *testing.T) {
	src := "- A\n  - B1\n  - B2\n\n  more A\n- [ ] todo\n- [x] done &amp; dusted\n\n" +
		"<!-- -->\n\n3. three\n4. four\n"
	doc := Parse([]byte(src), nil)
	if len(doc.Blocks) != 2 {
		t.Fatalf("got %d blocks, want 2", len(doc.Blocks))
	}

	ul := doc.Blocks[0].(*ListBlock)
	want := &ListBlock{Items: []ListItem{
		{Text: "A\n\nmore A", Nested: []*ListBlock{{Items: []ListItem{{Text: "B1"}, {Text: "B2"}}}}},
		{Text: "□ todo"},
		{Text: "■ done & dusted"},
	}}
	if !reflect.DeepEqual(withoutSpans(ul), want) {
		t.Errorf("unordered list:\n got %+v\nwant %+v", ul, want)
	}

	ol := doc.Blocks[1].(*ListBlock)
	if !ol.Ordered || ol.Start != 3 || ol.marker(0) != "3. " || ol.marker(1) != "4. " {
		t.Errorf("ordered list starts at %d, markers %q %q; want 3. and 4.", ol.Start, ol.marker(0), ol.marker(1))
	}
}

// withoutSpans clears the styled runs in l, leaving its text to compare.
func withoutSpans(l *ListBlock) *ListBlock {
	for i := range l.Items {
		l.Items[i].Spans = nil
		for _, n := range l.Items[i].Nested {
			withoutSpans(n)
		}
	}
	return l
}

// Inline text keeps its style: emphasis, strong, code, strikethrough and
// links, nested in each other. The runs of one link share it, and the
// document lists each link once.
func TestInlineSpans(t *testing.T) {
	doc := Parse([]byte("Plain *it* **bold `code`** ~~gone~~ [a **b**](https://x.org) <https://y.org>\n\n"+
		"- item [l](other.md)\n\n> quote *q*\n\n| `c` | [t](#top) |\n|---|---|\n| 1 | 2 |\n"), nil)
	p := doc.Blocks[0].(*ParagraphBlock)
	link := p.Spans[8].Link
	want := []Span{
		{Text: "Plain "},
		{Text: "it", Style: Italic},
		{Text: " "},
		{Text: "bold ", Style: Bold},
		{Text: "code", Style: Bold | Code},
		{Text: " "},
		{Text: "gone", Style: Strike},
		{Text: " "},
		{Text: "a ", Link: link},
		{Text: "b", Style: Bold, Link: link},
		{Text: " "},
		{Text: "https://y.org", Link: p.Spans[11].Link},
	}
	if !reflect.DeepEqual(p.Spans, want) {
		t.Errorf("paragraph:\n got %+v\nwant %+v", p.Spans, want)
	}
	if p.Text != "Plain it bold code gone a b https://y.org" {
		t.Errorf("paragraph text %q", p.Text)
	}
	if link == nil || link.URL != "https://x.org" || p.Spans[11].Link.URL != "https://y.org" {
		t.Fatalf("links %+v %+v", link, p.Spans[11].Link)
	}

	if s := doc.Blocks[1].(*ListBlock).Items[0].Spans; len(s) != 2 || s[1].Link == nil || s[1].Link.URL != "other.md" {
		t.Errorf("list item: %+v", s)
	}
	if s := doc.Blocks[2].(*QuoteBlock).Spans; len(s) != 2 || s[1] != (Span{Text: "q", Style: Italic}) {
		t.Errorf("quote: %+v", s)
	}
	tb := doc.Blocks[3].(*TableBlock)
	if c := tb.HeaderSpans[0]; len(c) != 1 || c[0].Style != Code {
		t.Errorf("table header: %+v", tb.HeaderSpans)
	}

	var urls []string
	for _, l := range doc.Links {
		urls = append(urls, l.URL)
	}
	if want := []string{"https://x.org", "https://y.org", "other.md", "#top"}; !reflect.DeepEqual(urls, want) {
		t.Errorf("links %q, want %q", urls, want)
	}
}

// Lists inside a quote keep their markers and nesting.
func TestQuoteWithList(t *testing.T) {
	doc := Parse([]byte("> Steps:\n>\n> 1. one\n>    - sub\n> 2. two\n"), nil)
	want := "Steps:\n1. one\n    • sub\n2. two"
	if got := doc.Blocks[0].(*QuoteBlock).Text; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// Each block knows the line it starts on, so a line leads to its block. A
// rule, which has no text, starts where the next block does.
func TestBlockLines(t *testing.T) {
	src := "# Title\n\nOne\ntwo\n\n---\n\n```go\nx\n```\n\n- a\n- b\n"
	d := Parse([]byte(src), nil)
	want := []int{1, 3, 8, 8, 12}
	if !reflect.DeepEqual(d.Lines, want) {
		t.Fatalf("lines %v, want %v", d.Lines, want)
	}
	for line, block := range map[int]int{1: 0, 2: 0, 4: 1, 6: 1, 8: 3, 10: 3, 13: 4, 99: 4} {
		if got := d.BlockAt(line); got != block {
			t.Errorf("line %d in block %d, want %d", line, got, block)
		}
	}
	if got := Parse(nil, nil).BlockAt(1); got != -1 {
		t.Errorf("empty document: block %d", got)
	}
}
