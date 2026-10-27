package app

import (
	"errors"
	"fmt"
	"image"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"dyamark/fonts"

	"gioui.org/app"
	"gioui.org/io/key"
	"gioui.org/io/system"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/text"
	"gioui.org/unit"
	"gioui.org/widget/material"
)

// Options holds configuration passed from main/CLI flags.
type Options struct {
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
	view   *SpikeView
	opts   Options
	ops    op.Ops

	themeWatcher *ThemeWatcher
	// fullShaper delivers a shaper that also has the system fonts.
	fullShaper chan *text.Shaper

	mu    sync.Mutex
	theme *Theme
}

// New creates a new application instance.
func New(opts Options) (*App, error) {
	// Match app ID to dyamark.desktop on Wayland / Hyprland
	app.ID = "dyamark"

	w := new(app.Window)
	w.Option(
		app.Title("dyamark"),
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

	a := &App{
		window: w,
		mTheme: mTheme,
		view:   &SpikeView{},
		opts:   opts,
		theme:  DefaultTheme(rounding),

		fullShaper: make(chan *text.Shaper, 1),
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
	firstFrameDone := false

	for {
		switch e := a.window.Event().(type) {
		case app.DestroyEvent:
			return e.Err

		case app.FrameEvent:
			gtx := app.NewContext(&a.ops, e)
			select {
			case s := <-a.fullShaper:
				a.mTheme.Shaper = s
			default:
			}

			for {
				ev, ok := gtx.Event(key.Filter{Required: key.ModCtrl, Name: "Q"})
				if !ok {
					break
				}
				if ke, ok := ev.(key.Event); ok && ke.State == key.Press {
					// Close through the window, so later phases can ask
					// about unsaved work before it goes.
					a.window.Perform(system.ActionClose)
				}
			}

			theme := a.currentTheme()
			paint.FillShape(&a.ops, theme.Background, clip.Rect(image.Rectangle{Max: e.Size}).Op())
			a.view.Layout(gtx, a.mTheme, theme)
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
