package markdown

import (
	"strings"

	"github.com/yuin/goldmark/ast"
)

// countWords counts the words a reader sees in doc: its text and code,
// without the markup, and without diagrams, which aren't prose.
func countWords(doc ast.Node, source []byte) int {
	var b strings.Builder
	ast.Walk(doc, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		// Blocks, table cells among them, part the words on either side.
		if n.Type() == ast.TypeBlock {
			b.WriteByte(' ')
		}
		if !entering {
			return ast.WalkContinue, nil
		}
		switch n := n.(type) {
		case *ast.Text:
			// Text runs split by markup, as in foo**bar**, are one word.
			b.Write(n.Segment.Value(source))
			if n.SoftLineBreak() || n.HardLineBreak() {
				b.WriteByte(' ')
			}
		case *ast.String:
			b.Write(n.Value)
		case *ast.AutoLink:
			b.Write(n.Label(source))
		case *ast.FencedCodeBlock:
			lang := strings.TrimSpace(string(n.Language(source)))
			code := extractCodeLines(n.Lines(), source)
			if isSequenceDiagram(lang, code) || isMermaidDiagram(lang, code) {
				return ast.WalkSkipChildren, nil
			}
			b.WriteString(code)
		case *ast.CodeBlock:
			b.WriteString(extractCodeLines(n.Lines(), source))
		case *ast.HTMLBlock, *ast.RawHTML:
			return ast.WalkSkipChildren, nil
		}
		return ast.WalkContinue, nil
	})
	return len(strings.Fields(b.String()))
}
