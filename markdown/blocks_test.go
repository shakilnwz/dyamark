package markdown

import (
	"image"
	"runtime"
	"testing"

	"gioui.org/font"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"
	"gioui.org/widget/material"
)

const longText = "lorem ipsum dolor sit amet consectetur adipiscing elit sed do"

// blockHeight lays out b as the view's list does, width wide with no
// height limit, and returns its height.
func blockHeight(t *testing.T, b Block, width int) int {
	t.Helper()
	th, mTheme := testThemes(t)
	gtx := layout.Context{
		Ops:         new(op.Ops),
		Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
		Constraints: layout.Constraints{Min: image.Pt(width, 0), Max: image.Pt(width, 1e6)},
	}
	return b.Layout(gtx, th, mTheme).Size.Y
}

// textWidth returns the width of s on one line in lbl's style.
func textWidth(t *testing.T, style func(th *material.Theme) material.LabelStyle) int {
	t.Helper()
	th, _ := testThemes(t)
	gtx := layout.Context{
		Ops:         new(op.Ops),
		Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
		Constraints: layout.Constraints{Max: image.Pt(1e6, 1e6)},
	}
	return style(th).Layout(gtx).Size.X
}

// A code line too long for the block wraps, and the lines after it move
// down to make room.
func TestCodeBlockWrappedLine(t *testing.T) {
	short := blockHeight(t, &CodeBlock{Code: "a\nb\n"}, 300)
	long := blockHeight(t, &CodeBlock{Code: longText + "\nb\n"}, 300)
	if long <= short {
		t.Errorf("a wrapped line takes no extra room: %d <= %d", long, short)
	}
}

// List item text starts right of the bullet, so it wraps that much sooner.
func TestListItemWrapsInsideBlock(t *testing.T) {
	w := textWidth(t, func(th *material.Theme) material.LabelStyle {
		return material.Label(th, unit.Sp(14), longText)
	})
	// Fits the block's width, but not the width right of the bullet.
	width := w + 10
	short := blockHeight(t, &ListBlock{Items: []ListItem{{Text: "x"}}}, width)
	long := blockHeight(t, &ListBlock{Items: []ListItem{{Text: longText}}}, width)
	if long <= short {
		t.Errorf("item text laid out past the right edge: %d <= %d", long, short)
	}

	nested := &ListBlock{Items: []ListItem{{Text: "x", Nested: []*ListBlock{{Items: []ListItem{{Text: longText}}}}}}}
	nestedShort := &ListBlock{Items: []ListItem{{Text: "x", Nested: []*ListBlock{{Items: []ListItem{{Text: "x"}}}}}}}
	if blockHeight(t, nested, w+40) <= blockHeight(t, nestedShort, w+40) {
		t.Error("nested item text laid out past the right edge")
	}
}

func TestQuoteWrapsInsideBlock(t *testing.T) {
	w := textWidth(t, func(th *material.Theme) material.LabelStyle {
		lbl := material.Label(th, unit.Sp(14), longText)
		lbl.Font.Style = font.Italic
		return lbl
	})
	width := w + 10
	short := blockHeight(t, &QuoteBlock{Text: "x"}, width)
	long := blockHeight(t, &QuoteBlock{Text: longText}, width)
	if long <= short {
		t.Errorf("quote text laid out past the right edge: %d <= %d", long, short)
	}
}

// Cell text wraps at its column, and the row grows to hold it.
func TestTableCellWrapsInsideColumn(t *testing.T) {
	w := textWidth(t, func(th *material.Theme) material.LabelStyle {
		return material.Label(th, unit.Sp(13), longText)
	})
	// Two columns, each a little wider than the text but narrower than
	// the text and its padding.
	width := 2 * (w + 10)
	short := blockHeight(t, &TableBlock{Headers: []string{"a", "b"}, Rows: [][]string{{"x", "y"}}}, width)
	long := blockHeight(t, &TableBlock{Headers: []string{"a", "b"}, Rows: [][]string{{longText, "y"}}}, width)
	if long <= short {
		t.Errorf("cell text not wrapped at its column: %d <= %d", long, short)
	}
	longHeader := blockHeight(t, &TableBlock{Headers: []string{longText, "b"}, Rows: [][]string{{"x", "y"}}}, width+40)
	if longHeader <= short {
		t.Errorf("header text not wrapped at its column: %d <= %d", longHeader, short)
	}
}

// The mmdc PNG is turned into an image op once, not copied and uploaded
// again every frame.
func TestMermaidImageOpReused(t *testing.T) {
	m := &MermaidOtherBlock{Rendered: image.NewNRGBA(image.Rect(0, 0, 1024, 1024))}
	th, mTheme := testThemes(t)
	gtx := layout.Context{
		Ops:         new(op.Ops),
		Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
		Constraints: layout.Constraints{Min: image.Pt(860, 0), Max: image.Pt(860, 1e6)},
	}
	frame := func() {
		gtx.Reset()
		m.Layout(gtx, th, mTheme)
	}
	frame()

	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	for range 5 {
		frame()
	}
	runtime.ReadMemStats(&after)
	// One copy of the image is 4 MB.
	if n := after.TotalAlloc - before.TotalAlloc; n > 2<<20 {
		t.Errorf("%d bytes allocated in 5 frames: the image is copied every frame", n)
	}
}

// Line breaks in styled text start new lines: a hard break in a
// paragraph, and a list item's second paragraph after a blank line.
func TestSpansBreakLines(t *testing.T) {
	height := func(src string) int {
		return blockHeight(t, Parse([]byte(src), nil).Blocks[0], 600)
	}
	one, two := height("one **two**\n"), height("one  \n**two**\n")
	if two < one*3/2 {
		t.Errorf("hard break: %d tall, one line is %d", two, one)
	}
	item, items := height("- one\n"), height("- one\n\n  two\n")
	if items < item*2 {
		t.Errorf("two paragraphs: %d tall, one is %d", items, item)
	}
}
