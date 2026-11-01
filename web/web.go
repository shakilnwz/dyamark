// Package web is dyamark's preview inside a web page, for the PhpStorm
// plugin: the Markdown view and the diagram viewer, driven by the page's
// calls and answering through a Host. cmd/dyamark-web binds it to
// JavaScript; this package builds and is tested natively.
package web

import (
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"sync"

	"dyamark/markdown"
	"dyamark/palette"
	"dyamark/seq"
	"dyamark/seq/parse"
	"dyamark/viewer"

	"gioui.org/io/event"
	"gioui.org/io/key"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/text"
	"gioui.org/widget/material"
)

// Host is what the preview tells the page. Its methods are called on the
// window's goroutine, while a frame is laid out.
type Host interface {
	// Ready is called with the first frame.
	Ready()
	// Drawn is called with the frame that first shows a new source.
	Drawn()
	// OpenLink asks to open a clicked link's URL.
	OpenLink(url string)
	// EditLine asks to show the source at a line, 1-based.
	EditLine(line int)
	// Scrolled tells the first line of the block at the top of the view,
	// 1-based, when scrolling in the view changes it; not when a new
	// source or ScrollToLine does.
	Scrolled(line int)
	// Log passes a message on for the IDE's log.
	Log(msg string)
}

// defaultRounding is the corners' rounding, in dp, until the page sets
// one.
const defaultRounding = 10

// Preview is the page's view of one document. SetSource, SetTheme and
// ScrollToLine may be called from any goroutine, before the first frame
// too; they take effect, in order, at the next frame. Layout, TopLine and
// Viewing are called on the window's goroutine.
type Preview struct {
	host       Host
	invalidate func()

	mu    sync.Mutex
	calls []func()
	// source is the newest source not shown yet, or nil.
	source *string

	th      *material.Theme
	theme   *palette.Theme
	mdTheme *markdown.Theme
	cache   *markdown.DiagramCache
	doc     *markdown.ParsedDocument
	view    *markdown.View
	viewer  *viewer.Viewer
	viewing bool
	// viewerPos is the place, among all the document's sequence diagrams,
	// of the one the viewer shows.
	viewerPos int

	ready bool
	// loaded is whether this frame shows a new source; moved whether a
	// new source or ScrollToLine moved the view.
	loaded, moved bool
	// topLine is the first line of the block at the top of the view, as
	// last told or set.
	topLine int
}

// New returns the preview of an empty document. shaper shapes its text,
// and invalidate asks the window for a frame.
func New(host Host, shaper *text.Shaper, invalidate func()) *Preview {
	th := material.NewTheme()
	th.Shaper = shaper
	cache := markdown.NewDiagramCache()
	doc := markdown.Parse(nil, cache)
	p := &Preview{
		host:       host,
		invalidate: invalidate,
		th:         th,
		cache:      cache,
		doc:        doc,
		view:       markdown.NewView(doc),
		viewer:     viewer.New(nil, 0),
	}
	p.setTheme(palette.Default(defaultRounding))

	p.view.OnOpenLink = host.OpenLink
	p.view.OnEditLine = host.EditLine
	p.view.OnOpenViewer = p.openViewer
	// No mmdc in a browser: other Mermaid diagrams show their source.
	p.view.OnRenderMermaid = nil
	p.viewer.OnClose = func() { p.viewing = false }
	p.viewer.OnEdit = func(int) {
		if pos := p.shownPos(); pos >= 0 {
			p.host.EditLine(p.doc.Sequences[pos].StartLine)
		}
	}
	return p
}

// SetSource shows text as the document. The newest of several calls
// between two frames is the one parsed.
func (p *Preview) SetSource(text string) {
	p.mu.Lock()
	queued := p.source != nil
	p.source = &text
	if !queued {
		p.calls = append(p.calls, p.takeSource)
	}
	p.mu.Unlock()
	p.invalidate()
}

// SetTheme sets the page's palette from data, a JSON Colors. It reports
// whether data is one, before the frame that uses it.
func (p *Preview) SetTheme(data string) error {
	t, err := ParseColors(data)
	if err != nil {
		return err
	}
	p.post(func() { p.setTheme(t) })
	return nil
}

// ScrollToLine puts the block holding line, 1-based, at the top of the
// view.
func (p *Preview) ScrollToLine(line int) {
	p.post(func() { p.scrollToLine(line) })
}

// post queues f for the next frame and asks for that frame.
func (p *Preview) post(f func()) {
	p.mu.Lock()
	p.calls = append(p.calls, f)
	p.mu.Unlock()
	p.invalidate()
}

// runCalls runs the calls queued since the last frame, in order.
func (p *Preview) runCalls() {
	p.mu.Lock()
	calls := p.calls
	p.calls = nil
	p.mu.Unlock()
	for _, f := range calls {
		f()
	}
}

func (p *Preview) takeSource() {
	p.mu.Lock()
	s := p.source
	p.source = nil
	p.mu.Unlock()
	if s != nil {
		p.load(*s)
	}
}

// load parses text and shows it, keeping the view's scroll and the
// viewer's diagram.
func (p *Preview) load(text string) {
	// The viewer's diagram is followed by its place among all the file's
	// diagrams: its index among those that parse shifts when one before
	// it breaks or is fixed.
	if pos := p.shownPos(); pos >= 0 {
		p.viewerPos = pos
	}
	p.doc = markdown.Parse([]byte(text), p.cache)
	// Only this text's diagrams stay cached, not each version typed.
	p.cache.Prune()
	p.view.SetDocument(p.doc)
	p.loaded, p.moved = true, true
	if p.viewing {
		p.followViewer()
	}
}

// followViewer gives the viewer the new document's diagrams, the way the
// app does on a reload.
func (p *Preview) followViewer() {
	asts, index := p.diagrams(p.viewerPos)
	switch {
	case len(asts) == 0:
	case index >= 0:
		p.viewer.ReloadDiagrams(asts, index)
	case p.viewerPos < len(p.doc.Sequences):
		// The diagram being shown doesn't parse right now; keep showing
		// its last good drawing until it does.
	default:
		p.viewer.SetDiagrams(asts, len(asts)-1)
	}
}

// diagrams returns the document's sequence diagrams that parse, and where
// the one at place pos among all of them is, or -1 if it doesn't parse.
func (p *Preview) diagrams(pos int) (asts []*parse.Diagram, index int) {
	index = -1
	for i, s := range p.doc.Sequences {
		if s.AST == nil {
			continue
		}
		if i == pos {
			index = len(asts)
		}
		asts = append(asts, s.AST)
	}
	return asts, index
}

// shownPos returns the place, among all the document's sequence
// diagrams, of the one the viewer shows, or -1 if it isn't one of them.
func (p *Preview) shownPos() int {
	for i, s := range p.doc.Sequences {
		if s.AST != nil && s.AST == p.viewer.AST {
			return i
		}
	}
	return -1
}

func (p *Preview) openViewer(b *markdown.SequenceBlock) {
	for i, s := range p.doc.Sequences {
		if s != b {
			continue
		}
		asts, index := p.diagrams(i)
		if index < 0 {
			return
		}
		p.viewerPos = i
		p.viewer.SetDiagrams(asts, index)
		p.viewing = true
		return
	}
}

func (p *Preview) setTheme(t *palette.Theme) {
	p.theme = t
	p.mdTheme = &markdown.Theme{
		Background:        t.Background,
		Foreground:        t.Foreground,
		Accent:            t.Accent,
		LighterBackground: t.LighterBackground,
		Error:             t.Error(),
		Rounding:          t.Rounding,
		Seq:               t.Seq,
	}
}

func (p *Preview) scrollToLine(line int) {
	i := p.doc.BlockAt(line)
	if i < 0 {
		return
	}
	p.view.List.Position = layout.Position{First: i}
	p.moved = true
}

// Layout runs the calls queued since the last frame and lays out the
// view, or the viewer when a diagram is open.
func (p *Preview) Layout(gtx layout.Context) layout.Dimensions {
	p.runCalls()
	for {
		ev, ok := gtx.Event(pointer.Filter{Target: p, Kinds: pointer.Press})
		if !ok {
			break
		}
		if e, ok := ev.(pointer.Event); ok && e.Kind == pointer.Press {
			// In a browser the keys go to Gio's hidden text field, which
			// has the page's focus only while Gio shows text input. A
			// click asks for it, so the keys typed after it arrive.
			gtx.Execute(key.SoftKeyboardCmd{Show: true})
		}
	}
	paint.FillShape(gtx.Ops, p.theme.Background, clip.Rect(image.Rectangle{Max: gtx.Constraints.Max}).Op())
	// The view and the viewer are laid out inside this area, so its
	// handler gets the presses on them too.
	area := clip.Rect(image.Rectangle{Max: gtx.Constraints.Max}).Push(gtx.Ops)
	event.Op(gtx.Ops, p)

	wasViewing := p.viewing
	var dims layout.Dimensions
	if p.viewing {
		dims = p.viewer.Layout(gtx, p.th, p.theme)
	} else {
		dims = p.view.Layout(gtx, p.th, p.mdTheme)
		if top := p.TopLine(); top != p.topLine {
			p.topLine = top
			if !p.moved {
				p.host.Scrolled(top)
			}
		}
	}
	area.Pop()
	if p.viewing != wasViewing {
		// The other one draws from the next frame.
		gtx.Execute(op.InvalidateCmd{})
	}

	if !p.ready {
		p.ready = true
		p.host.Ready()
	}
	if p.loaded {
		p.host.Drawn()
	}
	p.loaded, p.moved = false, false
	return dims
}

// TopLine returns the first line of the block at the top of the view,
// 1-based, or 0 when the document is empty.
func (p *Preview) TopLine() int {
	i := p.view.List.Position.First
	if i < 0 || i >= len(p.doc.Lines) {
		return 0
	}
	return p.doc.Lines[i]
}

// Viewing reports whether the viewer is open.
func (p *Preview) Viewing() bool {
	return p.viewing
}

// Colors is the palette the page sets, as JSON. Colours are "#rrggbb";
// rounding is the corners', in dp. What's left out keeps dyamark's
// default, but for the lighter background, which is derived from the
// background when only that is given.
type Colors struct {
	Background        string `json:"background"`
	Foreground        string `json:"foreground"`
	Accent            string `json:"accent"`
	LighterBackground string `json:"lighterBackground"`
	Error             string `json:"error"`
	Rounding          *int   `json:"rounding"`
}

// ParseColors returns the palette data sets: dyamark's default with the
// colours data gives, and the diagram theme derived from them.
func ParseColors(data string) (*palette.Theme, error) {
	var c Colors
	if err := json.Unmarshal([]byte(data), &c); err != nil {
		return nil, fmt.Errorf("theme: %w", err)
	}
	t := palette.Default(defaultRounding)
	if c.Rounding != nil {
		if *c.Rounding < 0 {
			return nil, fmt.Errorf("theme: rounding %d is negative", *c.Rounding)
		}
		t.Rounding = *c.Rounding
	}
	for _, f := range []struct {
		name, hex string
		to        *color.NRGBA
	}{
		{"background", c.Background, &t.Background},
		{"foreground", c.Foreground, &t.Foreground},
		{"accent", c.Accent, &t.Accent},
		{"lighterBackground", c.LighterBackground, &t.LighterBackground},
		{"error", c.Error, &t.Colors[1]},
	} {
		if f.hex == "" {
			continue
		}
		v, ok := seq.ParseHexColor(f.hex, 255)
		if !ok {
			return nil, fmt.Errorf("theme: %s %q isn't a #rrggbb colour", f.name, f.hex)
		}
		*f.to = v
	}
	// Only the background was given: derive a lighter one, as the app
	// does, so actor boxes still stand out from the page.
	if c.Background != "" && c.LighterBackground == "" {
		t.LighterBackground = palette.Mix(t.Background, t.Foreground, 0.08)
	}
	t.DeriveSeq()
	return t, nil
}
