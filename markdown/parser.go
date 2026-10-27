package markdown

import (
	"sort"
	"strings"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/text"
)

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

	doc := goldmark.DefaultParser().Parse(text.NewReader(source))
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
				startLine = lineIdx.LineNumber(n.Lines().At(0).Start)
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
		}
	}

	return &ParsedDocument{
		Blocks:    blocks,
		Sequences: sequences,
	}
}

func isSequenceDiagram(lang, code string) bool {
	trimmed := strings.TrimSpace(code)
	if strings.EqualFold(lang, "sequenceDiagram") {
		return true
	}
	if strings.EqualFold(lang, "mermaid") {
		return strings.HasPrefix(trimmed, "sequenceDiagram") ||
			strings.HasPrefix(trimmed, "%%{init") ||
			strings.HasPrefix(trimmed, "---")
	}
	return false
}

func isMermaidDiagram(lang, code string) bool {
	if strings.EqualFold(lang, "mermaid") {
		return true
	}
	return false
}

func detectMermaidType(code string) string {
	lines := strings.Split(strings.TrimSpace(code), "\n")
	for _, l := range lines {
		l = strings.TrimSpace(l)
		if strings.HasPrefix(l, "%%") || strings.HasPrefix(l, "---") {
			continue
		}
		fields := strings.Fields(l)
		if len(fields) > 0 {
			return fields[0]
		}
	}
	return "mermaid"
}

func extractCodeLines(lines *text.Segments, source []byte) string {
	var buf strings.Builder
	for i := 0; i < lines.Len(); i++ {
		seg := lines.At(i)
		buf.Write(seg.Value(source))
	}
	return buf.String()
}

func extractNodeText(n ast.Node, source []byte) string {
	var buf strings.Builder
	for child := n.FirstChild(); child != nil; child = child.NextSibling() {
		switch c := child.(type) {
		case *ast.Text:
			buf.Write(c.Segment.Value(source))
			if c.SoftLineBreak() {
				buf.WriteByte(' ')
			} else if c.HardLineBreak() {
				buf.WriteByte('\n')
			}
		case *ast.String:
			buf.Write(c.Value)
		case *ast.CodeSpan:
			buf.Write(c.Text(source))
		case *ast.Emphasis:
			buf.WriteString(extractNodeText(c, source))
		case *ast.Link:
			buf.WriteString(extractNodeText(c, source))
		case *ast.AutoLink:
			buf.Write(c.URL(source))
		default:
			if c.HasChildren() {
				buf.WriteString(extractNodeText(c, source))
			} else {
				buf.Write(c.Text(source))
			}
		}
	}
	return buf.String()
}

func extractList(l *ast.List, source []byte) *ListBlock {
	lb := &ListBlock{Ordered: l.IsOrdered()}
	for child := l.FirstChild(); child != nil; child = child.NextSibling() {
		if li, ok := child.(*ast.ListItem); ok {
			var itemText strings.Builder
			for sub := li.FirstChild(); sub != nil; sub = sub.NextSibling() {
				itemText.WriteString(extractNodeText(sub, source))
			}
			lb.Items = append(lb.Items, strings.TrimSpace(itemText.String()))
		}
	}
	return lb
}

func extractQuote(q *ast.Blockquote, source []byte) *QuoteBlock {
	var buf strings.Builder
	for child := q.FirstChild(); child != nil; child = child.NextSibling() {
		buf.WriteString(extractNodeText(child, source))
		buf.WriteString("\n")
	}
	return &QuoteBlock{Text: strings.TrimSpace(buf.String())}
}
