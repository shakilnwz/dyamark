package markdown

import (
	"bufio"
	"html"
	"sort"
	"strings"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	east "github.com/yuin/goldmark/extension/ast"
	gmhtml "github.com/yuin/goldmark/renderer/html"
	"github.com/yuin/goldmark/text"
)

var gm = goldmark.New(goldmark.WithExtensions(extension.GFM))

// ParsedDocument holds the rendered blocks and sequence diagrams from markdown.
type ParsedDocument struct {
	Blocks    []Block
	Sequences []*SequenceBlock
	// Lines holds the 1-based source line each block starts on.
	Lines []int
	// Links holds the links in the text, each once.
	Links []*Link
	// Words is how many words a reader sees: the text and code, not the
	// markup or the diagrams.
	Words int
}

// BlockAt returns the index of the block holding line, 1-based: the last
// block starting on or before it. It's -1 when there are no blocks.
func (d *ParsedDocument) BlockAt(line int) int {
	if len(d.Blocks) == 0 {
		return -1
	}
	i := sort.Search(len(d.Lines), func(i int) bool { return d.Lines[i] > line })
	return max(i-1, 0)
}

// LineIndex maps byte offsets to 1-based line numbers.
type LineIndex struct {
	lineStarts []int
}

// NewLineIndex builds a line index for source bytes.
func NewLineIndex(source []byte) *LineIndex {
	starts := []int{0}
	for i, b := range source {
		if b == '\n' {
			starts = append(starts, i+1)
		}
	}
	return &LineIndex{lineStarts: starts}
}

// LineNumber returns the 1-based line number for a byte offset.
func (idx *LineIndex) LineNumber(offset int) int {
	line := sort.Search(len(idx.lineStarts), func(i int) bool {
		return idx.lineStarts[i] > offset
	})
	if line < 1 {
		return 1
	}
	return line
}

// Parse converts a markdown document into a list of Block widgets.
func Parse(source []byte, cache *DiagramCache) *ParsedDocument {
	if cache == nil {
		cache = NewDiagramCache()
	}

	// A raw .mmd file holding a sequence diagram is drawn as one. Its
	// lines are numbered as in the file, not in the fenced copy.
	lineShift := 0
	trimmed := strings.TrimSpace(string(source))
	if isSequenceSource(trimmed) {
		lead := strings.Count(string(source[:strings.Index(string(source), trimmed)]), "\n")
		lineShift = lead - 1
		source = []byte("```mermaid\n" + trimmed + "\n```\n")
	}

	doc := gm.Parser().Parse(text.NewReader(source))
	lineIdx := NewLineIndex(source)

	var blocks []Block
	var sequences []*SequenceBlock
	var lines []int
	seqIndex := 0

	for node := doc.FirstChild(); node != nil; node = node.NextSibling() {
		count := len(blocks)
		switch n := node.(type) {
		case *ast.Heading:
			if spans := trimSpans(inlineSpans(n, source)); len(spans) > 0 {
				blocks = append(blocks, &HeadingBlock{Level: n.Level, Text: plainText(spans), Spans: spans})
			}

		case *ast.Paragraph:
			if spans := trimSpans(inlineSpans(n, source)); len(spans) > 0 {
				blocks = append(blocks, &ParagraphBlock{Text: plainText(spans), Spans: spans})
			}

		case *ast.FencedCodeBlock:
			lang := strings.TrimSpace(string(n.Language(source)))
			code := extractCodeLines(n.Lines(), source)
			startLine := 1
			if n.Lines().Len() > 0 {
				startLine = lineIdx.LineNumber(n.Lines().At(0).Start) + lineShift
			}

			if isSequenceDiagram(lang, code) {
				ast, diag, err, lastGood := cache.GetOrLayout(code, seqIndex)
				seqBlock := &SequenceBlock{
					Index:     seqIndex,
					Source:    code,
					StartLine: startLine,
					AST:       ast,
					Diagram:   diag,
					Error:     err,
					LastGood:  lastGood,
				}
				blocks = append(blocks, seqBlock)
				sequences = append(sequences, seqBlock)
				seqIndex++
			} else if isMermaidDiagram(lang, code) {
				diagType := detectMermaidType(code)
				blocks = append(blocks, &MermaidOtherBlock{
					DiagramType: diagType,
					Source:      code,
					StartLine:   startLine,
					Cache:       cache,
					// Rendered before this parse, say before a save:
					// still rendered.
					Rendered: cache.Rendered(code),
				})
			} else {
				blocks = append(blocks, &CodeBlock{Language: lang, Code: code})
			}

		case *ast.CodeBlock:
			code := extractCodeLines(n.Lines(), source)
			blocks = append(blocks, &CodeBlock{Code: code})

		case *ast.List:
			blocks = append(blocks, extractList(n, source))

		case *ast.Blockquote:
			blocks = append(blocks, extractQuote(n, source))

		case *ast.ThematicBreak:
			blocks = append(blocks, &DividerBlock{})

		case *east.Table:
			blocks = append(blocks, extractTable(n, source))
		}
		if len(blocks) > count {
			line := nodeLine(node, lineIdx)
			if line > 0 {
				line = max(line+lineShift, 1)
			}
			lines = append(lines, line)
		}
	}
	// A rule has no text to place it: it starts where the next block does,
	// so the lines before it stay with the block above.
	next := len(lineIdx.lineStarts)
	for i := len(lines) - 1; i >= 0; i-- {
		if lines[i] == 0 {
			lines[i] = next
		}
		next = lines[i]
	}

	d := &ParsedDocument{
		Blocks:    blocks,
		Sequences: sequences,
		Lines:     lines,
		Words:     countWords(doc, source),
	}
	d.Links = d.links()
	return d
}

// nodeLine returns the 1-based line n starts on, from its own text or its
// first descendant's, or 0 when it holds none.
func nodeLine(n ast.Node, idx *LineIndex) int {
	switch n := n.(type) {
	case *ast.FencedCodeBlock:
		// The fence is the line before the code.
		if n.Info != nil {
			return idx.LineNumber(n.Info.Segment.Start)
		}
		if n.Lines().Len() > 0 {
			return idx.LineNumber(n.Lines().At(0).Start) - 1
		}
		return 0
	case *ast.Text:
		return idx.LineNumber(n.Segment.Start)
	}
	if n.Type() == ast.TypeBlock && n.Lines().Len() > 0 {
		return idx.LineNumber(n.Lines().At(0).Start)
	}
	for c := n.FirstChild(); c != nil; c = c.NextSibling() {
		if line := nodeLine(c, idx); line > 0 {
			return line
		}
	}
	return 0
}

func isSequenceDiagram(lang, code string) bool {
	if strings.EqualFold(lang, "sequenceDiagram") {
		return true
	}
	if strings.EqualFold(lang, "mermaid") {
		return isSequenceSource(code)
	}
	return false
}

// isSequenceSource reports whether Mermaid source is a sequence diagram.
// Like Mermaid's own detector, it looks at the start of the first statement.
func isSequenceSource(code string) bool {
	return strings.HasPrefix(mermaidHeader(code), "sequenceDiagram")
}

func isMermaidDiagram(lang, code string) bool {
	if strings.EqualFold(lang, "mermaid") {
		return true
	}
	return false
}

func detectMermaidType(code string) string {
	if fields := strings.Fields(mermaidHeader(code)); len(fields) > 0 {
		return fields[0]
	}
	return "mermaid"
}

// mermaidHeader returns the first statement of Mermaid source, the one that
// names the diagram type, skipping front matter, %%{init}%% directives,
// %% comments and blank lines.
func mermaidHeader(code string) string {
	lines := strings.Split(code, "\n")
	i := 0
	for i < len(lines) && strings.TrimSpace(lines[i]) == "" {
		i++
	}
	// Front matter can only open the diagram.
	if i < len(lines) && strings.TrimSpace(lines[i]) == "---" {
		for i++; i < len(lines) && strings.TrimSpace(lines[i]) != "---"; i++ {
		}
		i++
	}
	for ; i < len(lines); i++ {
		l := strings.TrimSpace(lines[i])
		switch {
		case l == "":
		case strings.HasPrefix(l, "%%{"):
			// A directive runs to its closing }%%, maybe lines later.
			for !strings.Contains(lines[i], "}%%") && i+1 < len(lines) {
				i++
			}
		case strings.HasPrefix(l, "%%"):
		default:
			return l
		}
	}
	return ""
}

func extractCodeLines(lines *text.Segments, source []byte) string {
	var buf strings.Builder
	for i := 0; i < lines.Len(); i++ {
		seg := lines.At(i)
		buf.Write(seg.Value(source))
	}
	return buf.String()
}

// resolveText turns backslash escapes and entity references in Markdown
// text into the characters they stand for. goldmark's HTML writer does
// both in one pass, so `\&amp;` stays "&amp;"; its HTML-escaped output is
// then unescaped.
func resolveText(v []byte) string {
	var buf strings.Builder
	w := bufio.NewWriter(&buf)
	gmhtml.DefaultWriter.Write(w, v)
	w.Flush()
	return html.UnescapeString(buf.String())
}

func extractList(l *ast.List, source []byte) *ListBlock {
	lb := &ListBlock{Ordered: l.IsOrdered(), Start: l.Start}
	for child := l.FirstChild(); child != nil; child = child.NextSibling() {
		li, ok := child.(*ast.ListItem)
		if !ok {
			continue
		}
		var item ListItem
		if first := li.FirstChild(); first != nil {
			if box, ok := first.FirstChild().(*east.TaskCheckBox); ok {
				item.Task, item.Done = true, box.IsChecked
			}
		}
		for sub := li.FirstChild(); sub != nil; sub = sub.NextSibling() {
			if nested, ok := sub.(*ast.List); ok {
				item.Nested = append(item.Nested, extractList(nested, source))
				continue
			}
			spans := trimSpans(blockSpans(sub, source))
			if len(spans) == 0 {
				continue
			}
			if len(item.Spans) > 0 {
				item.Spans = append(item.Spans, Span{Text: "\n\n"})
			}
			item.Spans = append(item.Spans, spans...)
		}
		item.Text = plainText(item.Spans)
		lb.Items = append(lb.Items, item)
	}
	return lb
}

// blockText returns the plain text of a block inside a list item or quote.
func blockText(n ast.Node, source []byte) string {
	switch n := n.(type) {
	case *ast.FencedCodeBlock:
		return extractCodeLines(n.Lines(), source)
	case *ast.CodeBlock:
		return extractCodeLines(n.Lines(), source)
	case *ast.List:
		return extractList(n, source).plainText("")
	case *ast.Blockquote:
		return extractQuote(n, source).Text
	default:
		return plainText(inlineSpans(n, source))
	}
}

// blockSpans returns the text of a block inside a list item or quote:
// styled runs of a paragraph's text, one plain run of anything else.
func blockSpans(n ast.Node, source []byte) []Span {
	switch n := n.(type) {
	case *ast.FencedCodeBlock, *ast.CodeBlock, *ast.List:
		if txt := blockText(n, source); txt != "" {
			return []Span{{Text: txt}}
		}
		return nil
	case *ast.Blockquote:
		return extractQuote(n, source).Spans
	default:
		return inlineSpans(n, source)
	}
}

// plainText returns the list as lines of text, each item after its marker
// and nested lists indented under their item.
func (l *ListBlock) plainText(indent string) string {
	var buf strings.Builder
	for i, item := range l.Items {
		box := ""
		if item.Task {
			// □ and ■ are in the bundled font, so they show from the
			// first frame; ☐ and ☑ would wait for the system fonts.
			box = "□ "
			if item.Done {
				box = "■ "
			}
		}
		buf.WriteString(indent + l.marker(i) + box + item.Text + "\n")
		for _, n := range item.Nested {
			buf.WriteString(n.plainText(indent + "    "))
		}
	}
	return buf.String()
}

func extractQuote(q *ast.Blockquote, source []byte) *QuoteBlock {
	var spans []Span
	for child := q.FirstChild(); child != nil; child = child.NextSibling() {
		sub := blockSpans(child, source)
		for len(sub) > 0 {
			last := &sub[len(sub)-1]
			if last.Text = strings.TrimRight(last.Text, "\n"); last.Text != "" {
				break
			}
			sub = sub[:len(sub)-1]
		}
		spans = append(append(spans, sub...), Span{Text: "\n"})
	}
	spans = trimSpans(spans)
	return &QuoteBlock{Text: plainText(spans), Spans: spans}
}

func extractTable(t *east.Table, source []byte) *TableBlock {
	tb := &TableBlock{}
	for child := t.FirstChild(); child != nil; child = child.NextSibling() {
		switch r := child.(type) {
		case *east.TableHeader:
			for rowChild := r.FirstChild(); rowChild != nil; rowChild = rowChild.NextSibling() {
				if cell, ok := rowChild.(*east.TableCell); ok {
					spans := trimSpans(inlineSpans(cell, source))
					tb.Headers = append(tb.Headers, plainText(spans))
					tb.HeaderSpans = append(tb.HeaderSpans, spans)
				}
			}
		case *east.TableRow:
			var row []string
			var rowSpans [][]Span
			for cellChild := r.FirstChild(); cellChild != nil; cellChild = cellChild.NextSibling() {
				if cell, ok := cellChild.(*east.TableCell); ok {
					spans := trimSpans(inlineSpans(cell, source))
					row = append(row, plainText(spans))
					rowSpans = append(rowSpans, spans)
				}
			}
			tb.Rows = append(tb.Rows, row)
			tb.RowSpans = append(tb.RowSpans, rowSpans)
		}
	}
	return tb
}
