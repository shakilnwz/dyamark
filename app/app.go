package app

import (
	"errors"
	"fmt"
	"image"
	"io"
	"io/fs"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"dyamark/fonts"
	"dyamark/markdown"
	"dyamark/seq/parse"

	"gioui.org/app"
	"gioui.org/io/event"
	"gioui.org/io/key"
	"gioui.org/io/system"
	"gioui.org/io/transfer"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/text"
	"gioui.org/unit"
	"gioui.org/widget/material"
)

// Mode represents whether the app is in document view mode or fullscreen viewer mode.
type Mode int

const (
	ModeInline Mode = iota
	ModeViewer
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
	viewer *Viewer
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
	theme *Theme
	// pending holds work from other goroutines that touches the document
	// or views; it runs on the UI goroutine at the start of the next frame.
	pending []func()
}

// DefaultSampleMarkdown is shown when dyamark is launched without arguments.
const DefaultSampleMarkdown = `# dyamark

A fast, native Markdown viewer with live Mermaid sequence diagrams.

## Authentication Flow

Click or press **Enter** on the sequence diagram below to open it in the fullscreen viewer with zoom, pan, and fit:

` + "```mermaid" + `
sequenceDiagram
title Authentication Sequence Flow
participant Alice as Alice
participant API Gateway as API Gateway
participant Database as Database

Alice->>API Gateway: POST /api/session
activate API Gateway
opt Valid JWT Credentials
    API Gateway->>Database: SELECT user, hash FROM users
    activate Database
    Database-->>API Gateway: 1 row returned
    deactivate Database
    Note over API Gateway: Verify Argon2 hash<br/>& issue token
    API Gateway-->>Alice: 200 OK (JWT Bearer)
end
deactivate API Gateway
` + "```" + `

## Shortcuts

- **Enter** or **Click**: Open diagram in fullscreen viewer
- **0** / **1** / **W** / **H**: Fit to window / 100% / Fit width / Fit height (in viewer)
- **+** / **-** or **Scroll**: Zoom in / out at pointer
- **N**: Toggle sequence numbers
- **Esc**: Close viewer
`

// New creates a new application instance.
func New(opts Options) (*App, error) {
	// Match app ID to dyamark.desktop on Wayland / Hyprland
	app.ID = "dyamark"

	title := "dyamark"
	if opts.FilePath != "" {
		title = fmt.Sprintf("%s — dyamark", filepath.Base(opts.FilePath))
	}

	w := new(app.Window)
	w.Option(
		app.Title(title),
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
	if opts.FilePath != "" {
		data, err := os.ReadFile(opts.FilePath)
		if err == nil {
			source = data
		} else {
			fmt.Fprintf(os.Stderr, "dyamark: opening %s: %v\n", opts.FilePath, err)
			source = []byte(fmt.Sprintf("# Error: Cannot open file\n\n`%s`\n\n%v\n", opts.FilePath, err))
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
	viewer := NewViewer(initialASTs, 0)

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

	a := &App{
		window: w,
		mTheme: mTheme,
		mdView: mdView,
		viewer: viewer,
		mode:   initMode,
		cache:  cache,
		doc:    doc,
		opts:   opts,
		theme:  DefaultTheme(rounding),

		fullShaper: make(chan *text.Shaper, 1),
	}

	mdView.OnOpenViewer = func(block *markdown.SequenceBlock) {
		var asts []*parse.Diagram
		selectedIdx := 0
		for _, seq := range a.doc.Sequences {
			if seq == block {
				selectedIdx = len(asts)
			}
			if seq.AST != nil {
				asts = append(asts, seq.AST)
			}
		}
		if len(asts) > 0 {
			a.viewer.SetDiagrams(asts, selectedIdx)
			a.mode = ModeViewer
			a.window.Invalidate()
		}
	}

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

	viewer.OnClose = func() {
		a.mode = ModeInline
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

func (a *App) setTheme(t *Theme) {
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

	a.opts.FilePath = filePath
	a.window.Option(app.Title(fmt.Sprintf("%s — dyamark", filepath.Base(filePath))))

	doc := markdown.Parse(data, a.cache)
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
	}

	ext := strings.ToLower(filepath.Ext(filePath))
	if (ext == ".mmd" || ext == ".mermaid" || (len(doc.Blocks) == 1 && len(doc.Sequences) == 1)) && len(asts) > 0 {
		a.mode = ModeViewer
	} else {
		a.mode = ModeInline
	}

	if a.fileWatcher != nil {
		a.fileWatcher.Close()
		a.fileWatcher = nil
	}
	a.fileWatcher, _ = a.watch(filePath)

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
			a.reload(newData)
		})
	})
	return fw, err
}

// reload replaces the document with newData, keeping the viewer's place.
func (a *App) reload(newData []byte) {
	newDoc := markdown.Parse(newData, a.cache)
	a.doc = newDoc
	a.mdView.SetDocument(newDoc)

	var asts []*parse.Diagram
	for _, s := range newDoc.Sequences {
		if s.AST != nil {
			asts = append(asts, s.AST)
		}
	}
	if len(asts) > 0 {
		a.viewer.SetDiagrams(asts, a.viewer.CurIndex)
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

func (a *App) currentTheme() *Theme {
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

		case app.FrameEvent:
			a.runPending()
			gtx := app.NewContext(&a.ops, e)
			select {
			case s := <-a.fullShaper:
				a.mTheme.Shaper = s
			default:
			}

			for {
				ev, ok := gtx.Event(
					key.Filter{Required: key.ModCtrl, Name: "Q"},
					transfer.TargetFilter{Target: a, Type: "text/uri-list"},
				)
				if !ok {
					break
				}
				switch ev := ev.(type) {
				case key.Event:
					if ev.State == key.Press {
						// Close through the window, so later phases can ask
						// about unsaved work before it goes.
						a.window.Perform(system.ActionClose)
					}
				case transfer.DataEvent:
					if ev.Type == "text/uri-list" {
						rc := ev.Open()
						data, err := io.ReadAll(rc)
						rc.Close()
						if err == nil {
							for _, line := range strings.Split(string(data), "\n") {
								line = strings.TrimSpace(line)
								if strings.HasPrefix(line, "file://") {
									path := strings.TrimPrefix(line, "file://")
									if u, err := url.PathUnescape(path); err == nil {
										path = u
									}
									_ = a.OpenFile(path)
									break
								} else if line != "" && !strings.HasPrefix(line, "#") {
									_ = a.OpenFile(line)
									break
								}
							}
						}
					}
				}
			}

			if a.mode == ModeInline && a.mdView != nil {
				for {
					ev, ok := gtx.Event(
						key.Filter{Required: key.ModCtrl, Name: "="},
						key.Filter{Required: key.ModCtrl, Name: "+"},
						key.Filter{Required: key.ModCtrl, Name: "-"},
						key.Filter{Required: key.ModCtrl, Name: "_"},
						key.Filter{Required: key.ModCtrl, Name: "0"},
					)
					if !ok {
						break
					}
					if ke, ok := ev.(key.Event); ok && ke.State == key.Press {
						switch ke.Name {
						case "=", "+":
							a.mdView.ZoomTextIn()
							a.window.Invalidate()
						case "-", "_":
							a.mdView.ZoomTextOut()
							a.window.Invalidate()
						case "0":
							a.mdView.ResetTextZoom()
							a.window.Invalidate()
						}
					}
				}
			}

			theme := a.currentTheme()
			paint.FillShape(&a.ops, theme.Background, clip.Rect(image.Rectangle{Max: e.Size}).Op())

			// Register whole window area for drag-and-drop file target
			dropArea := clip.Rect(image.Rectangle{Max: e.Size}).Push(gtx.Ops)
			event.Op(gtx.Ops, a)
			dropArea.Pop()

			if a.mode == ModeViewer && a.viewer != nil {
				a.viewer.Layout(gtx, a.mTheme, theme)
			} else if a.mdView != nil {
				mdTh := &markdown.Theme{
					Background:        theme.Background,
					Foreground:        theme.Foreground,
					Accent:            theme.Accent,
					LighterBackground: theme.LighterBackground,
					Rounding:          theme.Rounding,
					Seq:               theme.Seq,
				}
				a.mdView.Layout(gtx, a.mTheme, mdTh)
			}

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
