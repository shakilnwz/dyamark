package markdown

import (
	"image"
	"image/color"
	"strings"
	"unicode"

	"dyamark/fonts"

	"gioui.org/font"
	"gioui.org/gesture"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget/material"
	"gioui.org/x/styledtext"
	"github.com/yuin/goldmark/ast"
	east "github.com/yuin/goldmark/extension/ast"
)

// Style is how a run of inline text is set: a set of the flags below.
type Style uint8

const (
	Bold Style = 1 << iota
	Italic
	Code
	Strike
)

// Link is where a link goes, and the clicks on it.
type Link struct {
	URL   string
	click gesture.Click
}

// Span is a run of inline text in one style. Link is set inside a link;
// the runs of one link share it.
type Span struct {
	Text  string
	Style Style
	Link  *Link
}

// inlineSpans returns the text of n's inline children as styled runs.
func inlineSpans(n ast.Node, source []byte) []Span {
	var out []Span
	appendInline(&out, n, source, 0, nil)
	return out
}

// appendSpan adds text to out, joining it to the last run if that's in the
// same style.
func appendSpan(out *[]Span, text string, style Style, link *Link) {
	if text == "" {
		return
	}
	if k := len(*out) - 1; k >= 0 && (*out)[k].Style == style && (*out)[k].Link == link {
		(*out)[k].Text += text
		return
	}
	*out = append(*out, Span{Text: text, Style: style, Link: link})
}

// appendInline adds the text of n's children to out, in style and inside
// link, with their own styles added.
func appendInline(out *[]Span, n ast.Node, source []byte, style Style, link *Link) {
	for child := n.FirstChild(); child != nil; child = child.NextSibling() {
		switch c := child.(type) {
		case *ast.Text:
			v := c.Segment.Value(source)
			if c.IsRaw() {
				// Code span text: kept as written, its line ends as spaces.
				appendSpan(out, strings.ReplaceAll(string(v), "\n", " "), style, link)
			} else {
				appendSpan(out, resolveText(v), style, link)
			}
			if c.HardLineBreak() {
				appendSpan(out, "\n", style, link)
			} else if c.SoftLineBreak() {
				appendSpan(out, " ", style, link)
			}
		case *ast.String:
			appendSpan(out, string(c.Value), style, link)
		case *ast.AutoLink:
			appendSpan(out, string(c.Label(source)), style, &Link{URL: string(c.URL(source))})
		case *east.TaskCheckBox:
			// The list draws the box.
		case *ast.RawHTML:
			// Inline HTML isn't drawn.
		case *ast.Emphasis:
			s := Italic
			if c.Level >= 2 {
				s = Bold
			}
			appendInline(out, c, source, style|s, link)
		case *ast.CodeSpan:
			appendInline(out, c, source, style|Code, link)
		case *east.Strikethrough:
			appendInline(out, c, source, style|Strike, link)
		case *ast.Link:
			appendInline(out, c, source, style, &Link{URL: string(c.Destination)})
		default:
			// Images: their alt text is in their children.
			appendInline(out, c, source, style, link)
		}
	}
}

// plainText returns the runs' text.
func plainText(spans []Span) string {
	var buf strings.Builder
	for _, s := range spans {
		buf.WriteString(s.Text)
	}
	return buf.String()
}

// orPlain returns spans, or text as one plain run when there are none:
// blocks made from plain text draw it.
func orPlain(spans []Span, text string) []Span {
	if spans == nil && text != "" {
		return []Span{{Text: text}}
	}
	return spans
}

// trimSpans drops the space around the runs, and runs left empty.
func trimSpans(spans []Span) []Span {
	for len(spans) > 0 {
		spans[0].Text = strings.TrimLeftFunc(spans[0].Text, unicode.IsSpace)
		if spans[0].Text != "" {
			break
		}
		spans = spans[1:]
	}
	for len(spans) > 0 {
		last := &spans[len(spans)-1]
		last.Text = strings.TrimRightFunc(last.Text, unicode.IsSpace)
		if last.Text != "" {
			break
		}
		spans = spans[:len(spans)-1]
	}
	return spans
}

// layoutSpans draws spans wrapped to gtx's width, at size in col, each in
// base with its own style added, and returns their size. Code has a tint
// behind it, links are in the accent colour, underlined, and take clicks,
// and struck text is crossed out.
func layoutSpans(gtx layout.Context, th *material.Theme, oTheme *Theme, spans []Span, size unit.Sp, col color.NRGBA, base font.Font) layout.Dimensions {
	styles := make([]styledtext.SpanStyle, len(spans))
	for i, s := range spans {
		st := styledtext.SpanStyle{Font: base, Size: size, Color: col, Content: s.Text}
		if s.Style&Bold != 0 {
			st.Font.Weight = font.Bold
		}
		if s.Style&Italic != 0 {
			st.Font.Style = font.Italic
		}
		if s.Style&Code != 0 {
			// At the same size, Dyamark Mono's baseline matches the
			// sans text's: styledtext lines runs up by their tops.
			st.Font.Typeface = fonts.Mono
		}
		if s.Link != nil {
			st.Color = oTheme.Accent
		}
		styles[i] = st
	}
	// The code tint is drawn over the text: it's the text's own colour, so
	// it only shows around the letters.
	tint := withAlpha(oTheme.Foreground, 30)
	text := styledtext.Text(th.Shaper, styles...)
	return text.Layout(gtx, func(gtx layout.Context, i int, dims layout.Dimensions) {
		s := spans[i]
		w := dims.Size.X
		line := func(y int, c color.NRGBA) {
			t := max(gtx.Dp(1), 1)
			paint.FillShape(gtx.Ops, c, clip.Rect(image.Rect(0, y, w, y+t)).Op())
		}
		if s.Style&Code != 0 {
			pad := gtx.Dp(2)
			r := image.Rect(-pad, 0, w+pad, dims.Size.Y)
			paint.FillShape(gtx.Ops, tint, clip.UniformRRect(r, gtx.Dp(3)).Op(gtx.Ops))
		}
		if s.Style&Strike != 0 {
			line(dims.Baseline-gtx.Sp(size)*3/10, styles[i].Color)
		}
		if s.Link != nil {
			line(dims.Baseline+gtx.Dp(2), withAlpha(oTheme.Accent, 150))
			area := clip.Rect{Max: dims.Size}.Push(gtx.Ops)
			pointer.CursorPointer.Add(gtx.Ops)
			s.Link.click.Add(gtx.Ops)
			area.Pop()
		}
	})
}

// links returns the document's links, each once.
func (d *ParsedDocument) links() []*Link {
	var links []*Link
	seen := make(map[*Link]bool)
	add := func(spans []Span) {
		for _, s := range spans {
			if s.Link != nil && !seen[s.Link] {
				seen[s.Link] = true
				links = append(links, s.Link)
			}
		}
	}
	var list func(l *ListBlock)
	list = func(l *ListBlock) {
		for _, item := range l.Items {
			add(item.Spans)
			for _, n := range item.Nested {
				list(n)
			}
		}
	}
	for _, b := range d.Blocks {
		switch b := b.(type) {
		case *HeadingBlock:
			add(b.Spans)
		case *ParagraphBlock:
			add(b.Spans)
		case *ListBlock:
			list(b)
		case *QuoteBlock:
			add(b.Spans)
		case *TableBlock:
			for _, c := range b.HeaderSpans {
				add(c)
			}
			for _, row := range b.RowSpans {
				for _, c := range row {
					add(c)
				}
			}
		}
	}
	return links
}
