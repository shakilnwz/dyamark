package app

import (
	"fmt"
	"image"
	"os"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"dyamark/fonts"

	"gioui.org/app"
	"gioui.org/io/key"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/text"
	"gioui.org/unit"
	"gioui.org/widget/material"
)

// Options holds configuration passed from main/CLI flags.
type Options struct {
	Measure bool
	ExitAfter bool
}

// App represents the main application instance.
type App struct {
	window   *app.Window
	theme    *Theme
	mTheme   *material.Theme
	view     *SpikeView
	opts     Options
	ops      op.Ops

	startTime time.Time
	mu        sync.Mutex
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

	// Load bundled fonts
	fontCollection, err := fonts.Collection()
	if err != nil {
		return nil, fmt.Errorf("loading bundled fonts: %w", err)
	}

	shaper := text.NewShaper(text.WithCollection(fontCollection))
	mTheme := material.NewTheme()
	mTheme.Shaper = shaper

	curTheme := LoadTheme()

	a := &App{
		window:    w,
		theme:     curTheme,
		mTheme:    mTheme,
		view:      NewSpikeView(),
		opts:      opts,
		startTime: time.Now(),
	}

	// Watch theme changes live
	_, _ = WatchTheme(func(newTheme *Theme) {
		a.mu.Lock()
		a.theme = newTheme
		a.mu.Unlock()
		a.window.Invalidate()
	})

	return a, nil
}

// Run starts the main event loop.
func (a *App) Run() error {
	var firstFrameDone bool

	for {
		ev := a.window.Event()
		switch e := ev.(type) {
		case app.DestroyEvent:
			return e.Err

		case app.FrameEvent:
			gtx := app.NewContext(&a.ops, e)

			// Process key events
			for {
				ev, ok := gtx.Event(key.Filter{
					Required: key.ModCtrl,
					Name:     "Q",
				})
				if !ok {
					break
				}
				if ke, ok := ev.(key.Event); ok && ke.State == key.Press {
					os.Exit(0)
				}
			}

			// Clear & Fill window background with current theme background
			a.mu.Lock()
			curTheme := a.theme
			a.mu.Unlock()

			paint.FillShape(&a.ops, curTheme.Background, clip.Rect(image.Rectangle{Max: e.Size}).Op())

			// Layout the sequence diagram spike
			a.view.Layout(gtx, a.mTheme, curTheme)

			e.Frame(gtx.Ops)

			// First paint metrics
			if !firstFrameDone {
				firstFrameDone = true
				firstPaintDuration := time.Since(a.startTime)
				rssKB, _ := GetRSSKB()
				rssMB := float64(rssKB) / 1024.0

				if a.opts.Measure {
					var ms runtime.MemStats
					runtime.ReadMemStats(&ms)
					fmt.Printf("[METRICS] first_paint_ms=%.2f\n", float64(firstPaintDuration.Microseconds())/1000.0)
					fmt.Printf("[METRICS] rss_mb=%.2f (go_heap_mb=%.2f sys_mb=%.2f)\n", rssMB, float64(ms.Alloc)/1024/1024, float64(ms.Sys)/1024/1024)
					if a.opts.ExitAfter {
						go func() {
							time.Sleep(50 * time.Millisecond)
							os.Exit(0)
						}()
					}
				}
			}
		}
	}
}

// GetRSSKB reads VmRSS from /proc/self/status.
func GetRSSKB() (uint64, error) {
	data, err := os.ReadFile("/proc/self/status")
	if err != nil {
		return 0, err
	}
	lines := strings.Split(string(data), "\n")
	for _, line := range lines {
		if strings.HasPrefix(line, "VmRSS:") {
			fields := strings.Fields(line)
			if len(fields) >= 2 {
				val, err := strconv.ParseUint(fields[1], 10, 64)
				if err == nil {
					return val, nil
				}
			}
		}
	}
	return 0, fmt.Errorf("VmRSS field not found")
}
