package web

import (
	"fmt"
	"image"
	"slices"
	"strings"
	"testing"

	"dyamark/fonts"
	"dyamark/markdown"
	"dyamark/seq"

	"gioui.org/f32"
	"gioui.org/io/input"
	"gioui.org/io/key"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/text"
	"gioui.org/unit"
)

const fence = "```"

// fakeHost records what the preview tells the page, one line each.
type fakeHost struct{ got []string }

func (h *fakeHost) Ready()              { h.got = append(h.got, "ready") }
func (h *fakeHost) Drawn()              { h.got = append(h.got, "drawn") }
func (h *fakeHost) OpenLink(url string) { h.got = append(h.got, "link "+url) }
func (h *fakeHost) EditLine(line int)   { h.got = append(h.got, fmt.Sprint("edit ", line)) }
func (h *fakeHost) Scrolled(line int)   { h.got = append(h.got, fmt.Sprint("scroll ", line)) }
func (h *fakeHost) Log(msg string)      { h.got = append(h.got, "log "+msg) }

// take returns what was told since the last take.
func (h *fakeHost) take() []string {
	got := h.got
	h.got = nil
	return got
}

// testPreview returns a preview, its host, the number of frames asked
// for so far, a router feeding it and a function laying out one 800×600
// frame of it.
func testPreview(t *testing.T) (*Preview, *fakeHost, *int, *input.Router, func()) {
	t.Helper()
	collection, err := fonts.Collection()
	if err != nil {
		t.Fatal(err)
	}
	host := new(fakeHost)
	asked := new(int)
	p := New(host, text.NewShaper(text.WithCollection(collection), text.NoSystemFonts()), func() { *asked++ })
	r := new(input.Router)
	gtx := layout.Context{
		Ops:         new(op.Ops),
		Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
		Constraints: layout.Exact(image.Pt(800, 600)),
		Source:      r.Source(),
	}
	return p, host, asked, r, func() {
		gtx.Reset()
		p.Layout(gtx)
		r.Frame(gtx.Ops)
	}
}

func press(r *input.Router, frame func(), name key.Name) {
	r.Queue(key.Event{Name: name, State: key.Press})
	frame()
}

// longDoc returns a document of n sections, each a heading and a
// paragraph long enough to fill several lines.
func longDoc(n int) string {
	var b strings.Builder
	for i := range n {
		fmt.Fprintf(&b, "# Section %d\n\n%s\n\n", i, strings.Repeat("Some words to read. ", 40))
	}
	return b.String()
}

// diagramDoc has a heading on line 1 and sequence diagrams opening on
// lines 3 and 8.
const diagramDoc = "# Diagrams\n\n" +
	fence + "mermaid\nsequenceDiagram\nA->>B: one\n" + fence + "\n\n" +
	fence + "mermaid\nsequenceDiagram\nC->>D: two\n" + fence + "\n"

// Calls made before the first frame are kept, and applied in order with
// it.
func TestCallsBeforeFirstFrame(t *testing.T) {
	p, host, asked, _, frame := testPreview(t)
	doc := longDoc(10)
	p.SetSource(doc)
	if err := p.SetTheme(`{"background":"#ffffff","foreground":"#202020"}`); err != nil {
		t.Fatal(err)
	}
	p.ScrollToLine(9)
	if *asked != 3 {
		t.Errorf("three calls asked for %d frames, want 3", *asked)
	}
	frame()

	if got, want := host.take(), []string{"ready", "drawn"}; !slices.Equal(got, want) {
		t.Errorf("told %q, want %q", got, want)
	}
	if n := len(p.doc.Blocks); n != 20 {
		t.Errorf("shows %d blocks, want 20", n)
	}
	if got, want := p.theme.Background, seq.HexColor("#ffffff", 255); got != want {
		t.Errorf("background is %v, want %v", got, want)
	}
	// Line 9 is "# Section 2", the fifth block.
	if got := p.TopLine(); got != 9 {
		t.Errorf("top line is %d, want 9", got)
	}
}

// Of the sources sent between two frames, only the newest is parsed and
// drawn.
func TestNewestSourceWins(t *testing.T) {
	p, host, asked, _, frame := testPreview(t)
	frame()
	host.take()

	for i := range 5 {
		p.SetSource(fmt.Sprintf("# Version %d\n", i))
	}
	if *asked != 5 {
		t.Errorf("five sources asked for %d frames, want 5", *asked)
	}
	if n := len(p.calls); n != 1 {
		t.Errorf("five sources queued %d calls, want 1", n)
	}
	frame()
	if got, want := host.take(), []string{"drawn"}; !slices.Equal(got, want) {
		t.Errorf("told %q, want %q", got, want)
	}
	if h, ok := p.doc.Blocks[0].(*markdown.HeadingBlock); !ok || h.Text != "Version 4" {
		t.Errorf("shows %#v, want the heading \"Version 4\"", p.doc.Blocks[0])
	}
	frame()
	if got := host.take(); len(got) != 0 {
		t.Errorf("a frame with nothing new told %q", got)
	}
}

// Scrolling in the view tells the new top line; a new source or
// ScrollToLine moving the view doesn't, so the IDE isn't told back what it
// just did.
func TestScrolledTellsOnlyScrolling(t *testing.T) {
	p, host, _, r, frame := testPreview(t)
	p.SetSource(longDoc(20))
	frame()
	host.take()

	p.ScrollToLine(13)
	frame()
	if got := p.TopLine(); got != 13 {
		t.Errorf("ScrollToLine(13) put line %d at the top, want 13", got)
	}
	if got := host.take(); len(got) != 0 {
		t.Errorf("ScrollToLine told %q", got)
	}

	press(r, frame, key.NamePageDown)
	got := host.take()
	if len(got) != 1 || !strings.HasPrefix(got[0], "scroll ") {
		t.Fatalf("Page Down told %q, want one scroll", got)
	}
	if want := fmt.Sprint("scroll ", p.TopLine()); got[0] != want || p.TopLine() <= 13 {
		t.Errorf("Page Down told %q, top line is %d", got[0], p.TopLine())
	}

	// Typing above the view keeps it where it was.
	p.SetSource("# Intro\n\n" + longDoc(20))
	frame()
	if got, want := host.take(), []string{"drawn"}; !slices.Equal(got, want) {
		t.Errorf("a new source told %q, want %q", got, want)
	}
}

// Enter opens the viewer, E in it asks to edit the diagram's source, and
// Esc goes back to the view.
func TestViewerEditsTheDiagram(t *testing.T) {
	p, host, _, r, frame := testPreview(t)
	p.SetSource(diagramDoc)
	frame()
	host.take()

	p.view.FocusNext()
	p.view.FocusNext()
	press(r, frame, key.NameReturn)
	if !p.Viewing() {
		t.Fatal("Enter didn't open the viewer")
	}
	frame()
	if p.viewer.AST != p.doc.Sequences[1].AST {
		t.Errorf("the viewer shows %v, want the second diagram", p.viewer.AST)
	}

	press(r, frame, "E")
	want := fmt.Sprint("edit ", p.doc.Sequences[1].StartLine)
	if got := host.take(); !slices.Equal(got, []string{want}) {
		t.Errorf("E told %q, want %q", got, want)
	}
	if !p.Viewing() {
		t.Error("E closed the viewer; it stays open to follow the edit")
	}

	press(r, frame, key.NameEscape)
	if p.Viewing() {
		t.Error("Esc didn't close the viewer")
	}
}

// The viewer keeps its diagram while the source changes: when one before
// it breaks, and while it is broken itself.
func TestViewerFollowsEdits(t *testing.T) {
	p, _, _, r, frame := testPreview(t)
	p.SetSource(diagramDoc)
	frame()
	p.view.FocusNext()
	p.view.FocusNext()
	press(r, frame, key.NameReturn)
	frame()

	broken := strings.Replace(diagramDoc, "A->>B: one", "A-", 1)
	p.SetSource(broken)
	frame()
	if p.viewer.AST != p.doc.Sequences[1].AST {
		t.Error("the viewer lost its diagram when the one before it broke")
	}

	shown := p.viewer.AST
	p.SetSource(strings.Replace(diagramDoc, "C->>D: two", "C-", 1))
	frame()
	if p.viewer.AST != shown {
		t.Error("the viewer dropped its diagram's last good drawing while it was broken")
	}

	p.SetSource(strings.Replace(diagramDoc, "C->>D: two", "C->>D: three", 1))
	frame()
	if p.viewer.AST != p.doc.Sequences[1].AST {
		t.Error("the viewer didn't show its diagram once it was fixed")
	}
}

// The view's links and edits go to the host.
func TestViewTellsLinksAndEdits(t *testing.T) {
	p, host, _, _, frame := testPreview(t)
	p.SetSource("[home](https://example.com)\n")
	frame()
	host.take()
	p.view.OnOpenLink("https://example.com")
	p.view.OnEditLine(4)
	if got, want := host.take(), []string{"link https://example.com", "edit 4"}; !slices.Equal(got, want) {
		t.Errorf("told %q, want %q", got, want)
	}
}

// A click gives the keys somewhere to go: it asks Gio to show text input,
// which in a browser focuses its hidden text field. On a diagram too.
func TestClickOpensTextInput(t *testing.T) {
	p, _, _, r, frame := testPreview(t)
	p.SetSource(diagramDoc)
	frame()
	if r.TextInputState() == input.TextInputOpen {
		t.Fatal("text input open before a click")
	}
	for _, at := range []f32.Point{{X: 400, Y: 10}, {X: 400, Y: 120}} {
		r.Queue(
			pointer.Event{Kind: pointer.Press, Source: pointer.Mouse, Buttons: pointer.ButtonPrimary, Position: at},
			pointer.Event{Kind: pointer.Release, Source: pointer.Mouse, Position: at},
		)
		frame()
		if got := r.TextInputState(); got != input.TextInputOpen {
			t.Errorf("a click at %v left text input %v, want it open", at, got)
		}
	}
}

func TestParseColors(t *testing.T) {
	th, err := ParseColors(`{"background":"#fafafa","accent":"#0969da","error":"#cf222e","rounding":4,"dark":false}`)
	if err != nil {
		t.Fatal(err)
	}
	if th.Background != seq.HexColor("#fafafa", 255) || th.Accent != seq.HexColor("#0969da", 255) {
		t.Errorf("colours not set: %v %v", th.Background, th.Accent)
	}
	if th.Error() != seq.HexColor("#cf222e", 255) {
		t.Errorf("error colour is %v", th.Error())
	}
	if th.Rounding != 4 || th.Seq.Rounding != 4 {
		t.Errorf("rounding is %d, diagrams' %v, want 4", th.Rounding, th.Seq.Rounding)
	}
	// Left out, it keeps the default.
	if th.Foreground != seq.HexColor("#e6edf3", 255) {
		t.Errorf("foreground is %v, want the default", th.Foreground)
	}
	if th.Seq.ActorBorder != th.Accent {
		t.Error("the diagram theme isn't derived from the colours")
	}

	// A background alone gets a lighter one to match, not the dark
	// default.
	light, err := ParseColors(`{"background":"#ffffff","foreground":"#000000"}`)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := light.LighterBackground, seq.HexColor("#ebebeb", 255); got != want {
		t.Errorf("lighter background is %v, want %v", got, want)
	}

	for _, bad := range []string{`{"background":"white"}`, `{"rounding":-1}`, `not json`, `{"accent":7}`} {
		if _, err := ParseColors(bad); err == nil {
			t.Errorf("ParseColors(%s) took it", bad)
		}
	}
}

// A palette that isn't one is refused at once, and changes nothing.
func TestSetThemeRefusesBadColors(t *testing.T) {
	p, _, asked, _, frame := testPreview(t)
	frame()
	before := p.theme
	if err := p.SetTheme(`{"background":"nope"}`); err == nil {
		t.Error("SetTheme took a bad colour")
	}
	if *asked != 0 {
		t.Error("a refused theme asked for a frame")
	}
	frame()
	if p.theme != before {
		t.Error("a refused theme changed the palette")
	}
}
