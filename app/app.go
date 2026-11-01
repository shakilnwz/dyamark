package app

import (
	"errors"
	"fmt"
	"image"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"dyamark/fonts"
	"dyamark/markdown"
	"dyamark/palette"
	"dyamark/seq/parse"
	"dyamark/viewer"

	"gioui.org/app"
	"gioui.org/io/event"
	"gioui.org/io/key"
	"gioui.org/io/pointer"
	"gioui.org/io/system"
	"gioui.org/io/transfer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/text"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"
)

// Mode is what the window shows.
type Mode int

const (
	// ModeInline is the rendered document.
	ModeInline Mode = iota
	// ModeViewer is one diagram, fullscreen.
	ModeViewer
	// ModeEdit is the source.
	ModeEdit
	// ModeSplit is the source beside the rendered document.
	ModeSplit
)

// Options holds configuration passed from main/CLI flags.
type Options struct {
	FilePath  string
	Measure   bool
	ExitAfter bool
	// Start is when main began, the zero point for the start-up metrics.
	Start time.Time
	// StartRSSKB is the resident set size when main began.
	StartRSSKB uint64
}

// App represents the main application instance.
type App struct {
	window *app.Window
	mTheme *material.Theme
	mdView *markdown.View
	viewer *viewer.Viewer
	mode   Mode
	cache  *markdown.DiagramCache
	doc    *markdown.ParsedDocument
	opts   Options
	ops    op.Ops

	themeWatcher *ThemeWatcher
	fileWatcher  *FileWatcher
	// fullShaper delivers a shaper that also has the system fonts.
	fullShaper chan *text.Shaper

	mu    sync.Mutex
	theme *palette.Theme
	// pending holds work from other goroutines that touches the document
	// or views; it runs on the UI goroutine at the start of the next frame.
	pending []func()

	// viewerPos is the place, among all the document's sequence
	// diagrams, of the one the viewer shows.
	viewerPos int
	// backMode is the mode the viewer goes back to when it closes.
	backMode Mode

	// editor holds the document's text.
	editor widget.Editor
	// saved is the text as last read from or written to the file, and
	// parsed the text the view shows.
	saved, parsed string
	// dirty is set while the editor has edits that aren't saved.
	dirty bool
	// saveErr is why the last save failed, until one succeeds.
	saveErr string
	// followDoc and followBlock are the document and block the split view
	// last showed the caret in.
	followDoc   *markdown.ParsedDocument
	followBlock int
	// parseAt and saveAt are when the view is laid out again and the file
	// saved after typing stops; zero when nothing is waiting.
	parseAt, saveAt time.Time
	// conflict is the file's text when it changed on disk while the editor
	// had edits, until one of them is chosen.
	conflict *string
	// prompt is the question shown over the window, if any.
	prompt *Prompt
	// help is set while the key list is shown over the window.
	help bool
	// find is the find bar, shown above the toolbar.
	find finder
	// toolList holds the toolbar's buttons, and tip the one whose tooltip
	// is shown.
	toolList []*tool
	tip      *tool
	// fileEvent is what last happened to the file, for the toolbar:
	// Opened, Saved, Reloaded, or New for one not made yet.
	fileEvent string
	// notice is a message shown in the toolbar, or over the viewer, until
	// noticeUntil.
	notice      string
	noticeUntil time.Time
	// focusEditor asks for the editor to take the keyboard next frame.
	focusEditor bool
	// dialogOpen is set while a portal file dialog is up.
	dialogOpen bool
	// quitting is set once closing with unsaved work was agreed to.
	quitting bool
	title    string
}

// DefaultSampleMarkdown is shown when dyamark is launched without arguments.
const DefaultSampleMarkdown = `# dyamark

A fast, native Markdown viewer with live Mermaid sequence diagrams.

## Authentication Flow

Click or press **Enter** on the sequence diagram below to open it in the fullscreen viewer with zoom, pan, and fit:

` + "```mermaid" + `
sequenceDiagram
title Authentication Sequence Flow
participant Alice
participant API as API Gateway
participant Database

Alice->>API: POST /api/session
activate API
opt Valid JWT Credentials
    API->>Database: SELECT user, hash FROM users
    activate Database
    Database-->>API: 1 row returned
    deactivate Database
    Note over API: Verify Argon2 hash<br/>& issue token
    API-->>Alice: 200 OK (JWT Bearer)
end
deactivate API
` + "```" + `

## Shortcuts

- **Ctrl+E**: Edit the source / back to the view
- **Ctrl+\\**: Source and view side by side
- **Ctrl+S** / **Ctrl+Shift+S**: Save / save as
- **Ctrl+O**: Open a file
- **Enter** or **Click**: Open diagram in fullscreen viewer
- **0** / **1** / **W** / **H**: Fit to window / 100% / Fit width / Fit height (in viewer)
- **+** / **-** or **Scroll**: Zoom in / out at pointer
- **N**: Toggle sequence numbers
- **Click a participant**: Show only what it takes part in (in viewer)
- **Esc**: Show all again / close viewer
`

// New creates a new application instance.
func New(opts Options) (*App, error) {
	// Match app ID to dyamark.desktop on Wayland / Hyprland
	app.ID = "dyamark"

	w := new(app.Window)
	w.Option(
		app.Size(unit.Dp(900), unit.Dp(640)),
		app.MinSize(unit.Dp(450), unit.Dp(350)),
	)

	fontCollection, err := fonts.Collection()
	if err != nil {
		return nil, fmt.Errorf("loading bundled fonts: %w", err)
	}
	// Scanning the system fonts takes ~60 ms, so the first frames use the
	// bundled fonts alone; the system fonts, needed as fallback for scripts
	// the bundled ones don't cover, arrive a moment later.
	mTheme := material.NewTheme()
	mTheme.Shaper = text.NewShaper(text.WithCollection(fontCollection), text.NoSystemFonts())

	// hyprctl is asked once; rounding rarely changes and theme reloads
	// shouldn't start a process each time.
	rounding, _ := HyprlandRounding()

	cache := markdown.NewDiagramCache()
	source := []byte(DefaultSampleMarkdown)
	newFile := false
	if opts.FilePath != "" {
		data, err := os.ReadFile(opts.FilePath)
		switch {
		case err == nil:
			source = data
		case errors.Is(err, fs.ErrNotExist):
			// A file to be: it's made on the first save.
			source = nil
			newFile = true
		default:
			fmt.Fprintf(os.Stderr, "dyamark: opening %s: %v\n", opts.FilePath, err)
			source = []byte(fmt.Sprintf("# Error: Cannot open file\n\n`%s`\n\n%v\n", opts.FilePath, err))
			// Never save over a file that couldn't be read.
			opts.FilePath = ""
		}
	}

	doc := markdown.Parse(source, cache)
	mdView := markdown.NewView(doc)

	// Collect initial ASTs for the viewer
	var initialASTs []*parse.Diagram
	for _, s := range doc.Sequences {
		if s.AST != nil {
			initialASTs = append(initialASTs, s.AST)
		}
	}
	vw := viewer.New(initialASTs, 0)

	initMode := ModeInline
	if opts.FilePath != "" {
		ext := strings.ToLower(filepath.Ext(opts.FilePath))
		if (ext == ".mmd" || ext == ".mermaid") && len(initialASTs) > 0 {
			initMode = ModeViewer
		}
	}
	if len(doc.Blocks) == 1 && len(doc.Sequences) == 1 && len(initialASTs) > 0 {
		initMode = ModeViewer
	}
	if newFile {
		initMode = ModeEdit
	}

	a := &App{
		window: w,
		mTheme: mTheme,
		mdView: mdView,
		viewer: vw,
		mode:   initMode,
		cache:  cache,
		doc:    doc,
		opts:   opts,
		theme:  palette.Default(rounding),

		fullShaper: make(chan *text.Shaper, 1),
	}
	a.load(source)
	if newFile {
		a.fileEvent = "New"
	}
	a.parsed = string(source)
	a.focusEditor = a.editing()
	a.updateTitle()

	mdView.OnOpenViewer = func(block *markdown.SequenceBlock) {
		a.flushParse()
		var asts []*parse.Diagram
		selectedIdx := 0
		for i, seq := range a.doc.Sequences {
			if seq == block {
				selectedIdx = len(asts)
				a.viewerPos = i
			}
			if seq.AST != nil {
				asts = append(asts, seq.AST)
			}
		}
		if len(asts) > 0 {
			a.viewer.SetDiagrams(asts, selectedIdx)
			if a.mode != ModeViewer {
				a.backMode = a.mode
			}
			a.mode = ModeViewer
			a.window.Invalidate()
		}
	}

	mdView.OnOpenLink = a.openLink
	mdView.OnEditLine = a.editAt

	mdView.OnRenderMermaid = func(block *markdown.MermaidOtherBlock) {
		go func() {
			img, err := a.cache.RenderMmdc(block.Source)
			a.post(func() {
				if err != nil {
					block.RenderError = err.Error()
				} else {
					block.Rendered = img
					block.RenderError = ""
				}
			})
		}()
	}

	vw.OnClose = func() {
		a.mode = a.backMode
		a.focusEditor = a.editing()
		a.window.Invalidate()
	}
	vw.OnEdit = func(int) {
		pos := a.shownPos()
		if pos < 0 {
			return
		}
		if a.backMode != ModeSplit {
			a.backMode = ModeEdit
		}
		a.mode = a.backMode
		a.editAt(a.doc.Sequences[pos].StartLine)
		a.window.Invalidate()
	}

	if opts.FilePath != "" {
		a.fileWatcher, err = a.watch(opts.FilePath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "dyamark: cannot watch %s: %v\n", opts.FilePath, err)
		}
	}

	go func() {
		a.fullShaper <- text.NewShaper(text.WithCollection(fontCollection))
		a.window.Invalidate()
	}()

	if dir, err := OmarchyDir(); err == nil {
		a.theme = LoadTheme(filepath.Join(dir, "theme", "colors.toml"), rounding)
		a.themeWatcher, err = WatchTheme(dir, rounding, a.setTheme)
		if err != nil && !errors.Is(err, fs.ErrNotExist) {
			fmt.Fprintf(os.Stderr, "dyamark: not following theme changes: %v\n", err)
		}
	}

	return a, nil
}

func (a *App) setTheme(t *palette.Theme) {
	a.mu.Lock()
	a.theme = t
	a.mu.Unlock()
	a.window.Invalidate()
}

// OpenFile loads a file into the application, updates watchers, title, and views.
func (a *App) OpenFile(filePath string) error {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return err
	}

	a.cache.ClearLastGood()
	a.load(data)
	a.parsed = string(data)
	doc := markdown.Parse(data, a.cache)
	a.cache.Prune()
	a.doc = doc
	a.mdView.SetDocument(doc)

	var asts []*parse.Diagram
	for _, s := range doc.Sequences {
		if s.AST != nil {
			asts = append(asts, s.AST)
		}
	}
	if len(asts) > 0 {
		a.viewer.SetDiagrams(asts, 0)
		a.viewerPos = 0
	}

	ext := strings.ToLower(filepath.Ext(filePath))
	switch {
	case a.editing():
		// Editing goes on in the new file.
		a.focusEditor = true
	case (ext == ".mmd" || ext == ".mermaid" || (len(doc.Blocks) == 1 && len(doc.Sequences) == 1)) && len(asts) > 0:
		a.mode = ModeViewer
		a.backMode = ModeInline
	default:
		a.mode = ModeInline
	}

	a.setPath(filePath)
	a.window.Invalidate()
	return nil
}

// watch follows filePath and reloads the document when it changes on disk.
func (a *App) watch(filePath string) (*FileWatcher, error) {
	var fw *FileWatcher
	fw, err := WatchFile(filePath, func(newData []byte) {
		a.post(func() {
			// A reload already queued for a file that has since been
			// replaced by another must not overwrite it.
			if a.fileWatcher != fw {
				return
			}
			a.diskChanged(newData)
		})
	})
	return fw, err
}

// shownPos returns the place, among all the document's sequence
// diagrams, of the one the viewer shows, or -1 if it isn't one of them.
func (a *App) shownPos() int {
	for i, s := range a.doc.Sequences {
		if s.AST != nil && s.AST == a.viewer.AST {
			return i
		}
	}
	return -1
}

// reload shows newData as the document, keeping the viewer's place.
func (a *App) reload(newData []byte) {
	// The viewer's diagram is followed by its place among all the file's
	// diagrams: its index among those that parse shifts when one before
	// it breaks or is fixed.
	if pos := a.shownPos(); pos >= 0 {
		a.viewerPos = pos
	}

	a.parsed = string(newData)
	newDoc := markdown.Parse(newData, a.cache)
	// Only this text's diagrams stay cached, not each version typed.
	a.cache.Prune()
	a.doc = newDoc
	a.mdView.SetDocument(newDoc)

	var asts []*parse.Diagram
	newIndex := -1
	for i, s := range newDoc.Sequences {
		if s.AST == nil {
			continue
		}
		if i == a.viewerPos {
			newIndex = len(asts)
		}
		asts = append(asts, s.AST)
	}
	switch {
	case len(asts) == 0:
	case newIndex >= 0:
		a.viewer.ReloadDiagrams(asts, newIndex)
	case a.viewerPos < len(newDoc.Sequences):
		// The diagram being shown doesn't parse right now; keep showing
		// its last good drawing until it does.
	default:
		a.viewer.SetDiagrams(asts, len(asts)-1)
	}
}

// post queues f to run on the UI goroutine before the next frame.
func (a *App) post(f func()) {
	a.mu.Lock()
	a.pending = append(a.pending, f)
	a.mu.Unlock()
	a.window.Invalidate()
}

// runPending runs the work queued by post.
func (a *App) runPending() {
	a.mu.Lock()
	pending := a.pending
	a.pending = nil
	a.mu.Unlock()
	for _, f := range pending {
		f()
	}
}

func (a *App) currentTheme() *palette.Theme {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.theme
}

// Run starts the main event loop. It returns when the window is closed.
func (a *App) Run() error {
	if a.themeWatcher != nil {
		defer a.themeWatcher.Close()
	}
	// The file watcher is replaced when another file is opened, so close
	// whichever one is current at the end.
	defer func() {
		if a.fileWatcher != nil {
			a.fileWatcher.Close()
		}
	}()
	firstFrameDone := false

	for {
		switch e := a.window.Event().(type) {
		case app.DestroyEvent:
			return e.Err

		case *app.ClosingEvent:
			a.closing(e)

		case app.FrameEvent:
			gtx := app.NewContext(&a.ops, e)
			select {
			case s := <-a.fullShaper:
				a.mTheme.Shaper = s
			default:
			}
			a.frame(gtx)
			e.Frame(gtx.Ops)

			if !firstFrameDone {
				firstFrameDone = true
				if a.opts.Measure {
					a.reportMetrics()
				}
			}
		}
	}
}

// closing decides whether the window may close: edits to a file are saved
// first, and closing an untitled draft or one at odds with its file asks.
func (a *App) closing(e *app.ClosingEvent) {
	if a.quitting || !a.isDirty() {
		return
	}
	if a.opts.FilePath != "" && a.conflict == nil && a.saveTo(a.opts.FilePath) {
		return
	}
	e.Abort()
	a.whenSaved(a.quit)
	a.window.Invalidate()
}

// frame handles a frame's input and draws it.
func (a *App) frame(gtx layout.Context) {
	a.runPending()
	a.tick(gtx.Now)
	if a.prompt == nil && !a.help {
		a.shortcuts(gtx)
	}

	theme := a.currentTheme()
	size := gtx.Constraints.Max
	paint.FillShape(gtx.Ops, theme.Background, clip.Rect(image.Rectangle{Max: size}).Op())

	// Register whole window area for drag-and-drop file target
	dropArea := clip.Rect(image.Rectangle{Max: size}).Push(gtx.Ops)
	event.Op(gtx.Ops, a)
	dropArea.Pop()

	// Under a prompt or the key list the window takes no input.
	under := gtx
	if a.prompt != nil || a.help {
		under = gtx.Disabled()
	}
	// The toolbar takes the bottom of the window, but for the viewer's,
	// and the find bar the line above it.
	toolbarH, findH := 0, 0
	if a.mode != ModeViewer {
		toolbarH = gtx.Dp(toolbarHeight)
	}
	if a.find.open && a.mode != ModeViewer {
		findH = gtx.Dp(findHeight)
	}
	under.Constraints = layout.Exact(image.Pt(size.X, max(size.Y-toolbarH-findH, 0)))
	mdTh := &markdown.Theme{
		Background:        theme.Background,
		Foreground:        theme.Foreground,
		Accent:            theme.Accent,
		LighterBackground: theme.LighterBackground,
		Error:             errorColor(theme),
		Rounding:          theme.Rounding,
		Seq:               theme.Seq,
	}
	switch a.mode {
	case ModeViewer:
		a.viewer.Layout(under, a.mTheme, theme)
	case ModeEdit:
		a.layoutEditor(under, theme, true)
	case ModeSplit:
		a.layoutSplit(under, theme, mdTh)
	default:
		a.mdView.NoPlainKeys = gtx.Focused(&a.find.ed)
		a.mdView.Layout(under, a.mTheme, mdTh)
	}

	if findH > 0 {
		bar := under
		bar.Constraints = layout.Exact(image.Pt(size.X, findH))
		off := op.Offset(image.Pt(0, size.Y-toolbarH-findH)).Push(gtx.Ops)
		a.layoutFind(bar, theme)
		off.Pop()
	}
	if toolbarH > 0 {
		bar := under
		bar.Constraints = layout.Exact(image.Pt(size.X, toolbarH))
		off := op.Offset(image.Pt(0, size.Y-toolbarH)).Push(gtx.Ops)
		a.layoutToolbar(bar, theme)
		off.Pop()
	}
	if a.help {
		a.layoutHelp(gtx, theme)
	}
	if a.prompt != nil {
		a.layoutPrompt(gtx, theme)
	}
	// Elsewhere the toolbar says what the notice says.
	if a.notice != "" && a.mode == ModeViewer {
		above := gtx
		above.Constraints = layout.Exact(image.Pt(size.X, max(size.Y-toolbarH-findH, 0)))
		a.layoutNotice(above, theme)
	}
	// Wake up for the parse, save or notice waiting on typing to stop.
	if next := a.tick(gtx.Now); !next.IsZero() {
		gtx.Execute(op.InvalidateCmd{At: next})
	}
}

// layoutSplit draws the source on the left and the view on the right.
func (a *App) layoutSplit(gtx layout.Context, theme *palette.Theme, mdTh *markdown.Theme) {
	size := gtx.Constraints.Max
	half := size.X / 2
	left := gtx
	left.Constraints = layout.Exact(image.Pt(half, size.Y))
	a.layoutEditor(left, theme, false)

	paint.FillShape(gtx.Ops, palette.WithAlpha(theme.Foreground, 40),
		clip.Rect(image.Rect(half, 0, half+1, size.Y)).Op())

	right := gtx
	right.Constraints = layout.Exact(image.Pt(size.X-half-1, size.Y))
	off := op.Offset(image.Pt(half+1, 0)).Push(gtx.Ops)
	// A click in the view lets go of the editor, so the view's own keys
	// work again.
	for {
		ev, ok := gtx.Event(pointer.Filter{Target: &a.mdView, Kinds: pointer.Press})
		if !ok {
			break
		}
		if _, ok := ev.(pointer.Event); ok {
			gtx.Execute(key.FocusCmd{})
		}
	}
	area := clip.Rect(image.Rectangle{Max: right.Constraints.Max}).Push(gtx.Ops)
	event.Op(gtx.Ops, &a.mdView)
	a.mdView.NoPlainKeys = gtx.Focused(&a.editor) || gtx.Focused(&a.find.ed)
	// The view follows the caret to another block, and through edits,
	// while the editor has the keyboard.
	line, _ := a.editor.CaretPos()
	if b := a.mdView.Doc.BlockAt(line + 1); b != a.followBlock || a.mdView.Doc != a.followDoc {
		a.followDoc, a.followBlock = a.mdView.Doc, b
		if b >= 0 && gtx.Focused(&a.editor) {
			a.mdView.ShowBlock(b)
		}
	}
	a.mdView.Layout(right, a.mTheme, mdTh)
	area.Pop()
	off.Pop()
}

// shortcuts handles the keys that work everywhere, and dropped files.
func (a *App) shortcuts(gtx layout.Context) {
	filters := []event.Filter{
		key.Filter{Required: key.ModCtrl, Name: "Q"},
		key.Filter{Required: key.ModCtrl, Name: "O"},
		key.Filter{Required: key.ModCtrl, Optional: key.ModShift, Name: "S"},
		key.Filter{Required: key.ModCtrl, Name: "E"},
		key.Filter{Required: key.ModCtrl, Name: "\\"},
		key.Filter{Required: key.ModCtrl, Name: "F"},
		transfer.TargetFilter{Target: a, Type: "text/uri-list"},
	}
	filters = append(filters, a.helpKeys(gtx)...)
	if a.mode == ModeEdit {
		// The view, which handles text size, isn't laid out.
		filters = append(filters,
			key.Filter{Required: key.ModCtrl, Name: "="},
			key.Filter{Required: key.ModCtrl, Optional: key.ModShift, Name: "+"},
			key.Filter{Required: key.ModCtrl, Name: "-"},
			key.Filter{Required: key.ModCtrl, Optional: key.ModShift, Name: "_"},
			key.Filter{Required: key.ModCtrl, Name: "0"},
		)
	}
	for {
		ev, ok := gtx.Event(filters...)
		if !ok {
			break
		}
		switch ev := ev.(type) {
		case key.Event:
			if ev.State == key.Press {
				a.shortcut(ev)
			}
		case transfer.DataEvent:
			rc := ev.Open()
			data, err := io.ReadAll(rc)
			rc.Close()
			if err != nil {
				break
			}
			for line := range strings.Lines(string(data)) {
				line = strings.TrimSpace(line)
				if line == "" || strings.HasPrefix(line, "#") {
					continue
				}
				path := line
				if strings.HasPrefix(line, "file://") {
					if path, err = uriPath(line); err != nil {
						break
					}
				}
				a.whenSaved(func() { a.open(path) })
				break
			}
		}
	}
}

// shortcut acts on a key pressed with Ctrl.
func (a *App) shortcut(e key.Event) {
	switch e.Name {
	case "Q":
		// Close through the window, which asks about unsaved work.
		a.window.Perform(system.ActionClose)
	case "O":
		a.whenSaved(a.openDialog)
	case "S":
		if e.Modifiers.Contain(key.ModShift) {
			a.saveAs(nil)
		} else {
			a.save()
		}
	case "E":
		a.toggleSource()
	case "\\":
		a.toggleSplit()
	case "?", key.NameF1:
		a.help = true
	case "F":
		a.openFind()
	case "=", "+":
		a.mdView.ZoomTextIn()
	case "-", "_":
		a.mdView.ZoomTextOut()
	case "0":
		a.mdView.ResetTextZoom()
	}
	a.window.Invalidate()
}

// toggleSource shows the source, or the view again. From the viewer it
// edits the diagram shown.
func (a *App) toggleSource() {
	switch a.mode {
	case ModeEdit:
		a.setMode(ModeInline)
	case ModeViewer:
		a.viewer.OnEdit(a.viewer.CurIndex)
	default:
		a.setMode(ModeEdit)
	}
}

// toggleSplit shows the source beside the view, or the view alone again.
func (a *App) toggleSplit() {
	if a.mode == ModeSplit {
		a.setMode(ModeInline)
	} else {
		a.setMode(ModeSplit)
	}
}

// reportMetrics prints start-up time and memory after the first frame.
func (a *App) reportMetrics() {
	firstPaint := time.Since(a.opts.Start)
	rssKB, _ := RSSKB()
	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)
	mb := func(kb uint64) float64 { return float64(kb) / 1024 }
	fmt.Printf("[METRICS] first_paint_ms=%.2f\n", float64(firstPaint.Microseconds())/1000)
	fmt.Printf("[METRICS] rss_mb=%.2f (at_start_mb=%.2f go_heap_mb=%.2f go_sys_mb=%.2f)\n",
		mb(rssKB), mb(a.opts.StartRSSKB), float64(ms.Alloc)/1024/1024, float64(ms.Sys)/1024/1024)
	if a.opts.ExitAfter {
		a.window.Perform(system.ActionClose)
	}
}

// RSSKB reads VmRSS from /proc/self/status.
func RSSKB() (uint64, error) {
	data, err := os.ReadFile("/proc/self/status")
	if err != nil {
		return 0, err
	}
	for line := range strings.Lines(string(data)) {
		if rest, ok := strings.CutPrefix(line, "VmRSS:"); ok {
			fields := strings.Fields(rest)
			if len(fields) >= 1 {
				return strconv.ParseUint(fields[0], 10, 64)
			}
		}
	}
	return 0, fmt.Errorf("VmRSS field not found")
}
