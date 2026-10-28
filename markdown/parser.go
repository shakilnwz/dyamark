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
	seqIndex := 0

	for node := doc.FirstChild(); node != nil; node = node.NextSibling() {
		switch n := node.(type) {
		case *ast.Heading:
			txt := strings.TrimSpace(extractNodeText(n, source))
			if txt != "" {
				blocks = append(blocks, &HeadingBlock{Level: n.Level, Text: txt})
			}

		case *ast.Paragraph:
			txt := strings.TrimSpace(extractNodeText(n, source))
			if txt != "" {
				blocks = append(blocks, &ParagraphBlock{Text: txt})
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
	}

	return &ParsedDocument{
		Blocks:    blocks,
		Sequences: sequences,
	}
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

// extractNodeText returns the plain text of n's inline children.
func extractNodeText(n ast.Node, source []byte) string {
	var buf strings.Builder
	for child := n.FirstChild(); child != nil; child = child.NextSibling() {
		switch c := child.(type) {
		case *ast.Text:
			v := c.Segment.Value(source)
			if c.IsRaw() {
				// Code span text: kept as written, its line ends as spaces.
				buf.WriteString(strings.ReplaceAll(string(v), "\n", " "))
			} else {
				buf.WriteString(resolveText(v))
			}
			if c.HardLineBreak() {
				buf.WriteByte('\n')
			} else if c.SoftLineBreak() {
				buf.WriteByte(' ')
			}
		case *ast.String:
			buf.Write(c.Value)
		case *ast.AutoLink:
			buf.Write(c.Label(source))
		case *east.TaskCheckBox:
			// □ and ■ are in the bundled font, so the boxes show from the
			// first frame; ☐ and ☑ would wait for the system fonts.
			if c.IsChecked {
				buf.WriteString("■ ")
			} else {
				buf.WriteString("□ ")
			}
		case *ast.RawHTML:
			// Inline HTML isn't drawn.
		default:
			// Emphasis, links, images, strikethrough and code spans: their
			// text is in their children.
			buf.WriteString(extractNodeText(c, source))
		}
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
		var paras []string
		for sub := li.FirstChild(); sub != nil; sub = sub.NextSibling() {
			if nested, ok := sub.(*ast.List); ok {
				item.Nested = append(item.Nested, extractList(nested, source))
				continue
			}
			if txt := strings.TrimSpace(blockText(sub, source)); txt != "" {
				paras = append(paras, txt)
			}
		}
		item.Text = strings.Join(paras, "\n\n")
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
		return extractNodeText(n, source)
	}
}

// plainText returns the list as lines of text, each item after its marker
// and nested lists indented under their item.
func (l *ListBlock) plainText(indent string) string {
	var buf strings.Builder
	for i, item := range l.Items {
		buf.WriteString(indent + l.marker(i) + item.Text + "\n")
		for _, n := range item.Nested {
			buf.WriteString(n.plainText(indent + "    "))
		}
	}
	return buf.String()
}

func extractQuote(q *ast.Blockquote, source []byte) *QuoteBlock {
	var buf strings.Builder
	for child := q.FirstChild(); child != nil; child = child.NextSibling() {
		buf.WriteString(strings.TrimRight(blockText(child, source), "\n"))
		buf.WriteString("\n")
	}
	return &QuoteBlock{Text: strings.TrimSpace(buf.String())}
}

func extractTable(t *east.Table, source []byte) *TableBlock {
	tb := &TableBlock{}
	for child := t.FirstChild(); child != nil; child = child.NextSibling() {
		switch r := child.(type) {
		case *east.TableHeader:
			for rowChild := r.FirstChild(); rowChild != nil; rowChild = rowChild.NextSibling() {
				if cell, ok := rowChild.(*east.TableCell); ok {
					tb.Headers = append(tb.Headers, strings.TrimSpace(extractNodeText(cell, source)))
				}
			}
		case *east.TableRow:
			var row []string
			for cellChild := r.FirstChild(); cellChild != nil; cellChild = cellChild.NextSibling() {
				if cell, ok := cellChild.(*east.TableCell); ok {
					row = append(row, strings.TrimSpace(extractNodeText(cell, source)))
				}
			}
			tb.Rows = append(tb.Rows, row)
		}
	}
	return tb
}
