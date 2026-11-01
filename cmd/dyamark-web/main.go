//go:build js && wasm

// Command dyamark-web is dyamark's Markdown view built for WebAssembly, to
// run in the PhpStorm plugin's preview panel.
//
// This is the spike of step 1 in docs/plans/2026-11-01-phpstorm-plugin.md:
// just enough to see whether WebGL, typing and keys work inside PhpStorm.
// Step 3 replaces it.
//
// The page calls dyamark.setSource(text) with the whole document. dyamark
// answers through the page's dyamarkSend(message): "ready" after the first
// frame, "drawn" after the frame that shows a new source, then "link URL",
// "viewer LINE", "edit LINE", "render" and "error TEXT".
package main

import (
	"fmt"
	"image"
	"sync"
	"syscall/js"

	"dyamark/fonts"
	"dyamark/markdown"
	"dyamark/seq"

	"gioui.org/app"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/text"
	"gioui.org/widget/material"
)

func main() {
	w := new(app.Window)
	in := new(inbox)
	js.Global().Set("dyamark", js.ValueOf(map[string]any{
		"setSource": js.FuncOf(func(_ js.Value, args []js.Value) any {
			if len(args) > 0 {
				in.put(args[0].String())
				w.Invalidate()
			}
			return nil
		}),
	}))
	go run(w, in)
	app.Main()
}

// inbox holds the newest source from the page until the next frame.
type inbox struct {
	mu     sync.Mutex
	source *string
}

func (in *inbox) put(s string) {
	in.mu.Lock()
	in.source = &s
	in.mu.Unlock()
}

func (in *inbox) take() *string {
	in.mu.Lock()
	defer in.mu.Unlock()
	s := in.source
	in.source = nil
	return s
}

// send passes a message to the page, which passes it to the plugin.
func send(msg string) {
	if f := js.Global().Get("dyamarkSend"); f.Type() == js.TypeFunction {
		f.Invoke(msg)
	}
}

func run(w *app.Window, in *inbox) {
	th := material.NewTheme()
	faces, err := fonts.Collection()
	if err != nil {
		send("error fonts: " + err.Error())
	}
	th.Shaper = text.NewShaper(text.NoSystemFonts(), text.WithCollection(faces))

	// The app's fallback palette, until step 5 takes the IDE's colours.
	mdTh := &markdown.Theme{
		Background:        seq.HexColor("#0b0f19", 255),
		Foreground:        seq.HexColor("#e6edf3", 255),
		Accent:            seq.HexColor("#FFD60A", 255),
		LighterBackground: seq.HexColor("#1f242c", 255),
		Error:             seq.HexColor("#f85149", 255),
		Rounding:          8,
		Seq:               seq.DarkTheme(),
	}
	th.Palette.Bg = mdTh.Background
	th.Palette.Fg = mdTh.Foreground
	th.Palette.ContrastBg = mdTh.Accent

	cache := markdown.NewDiagramCache()
	view := markdown.NewView(markdown.Parse(nil, cache))
	view.OnOpenLink = func(url string) { send("link " + url) }
	view.OnEditLine = func(line int) { send(fmt.Sprintf("edit %d", line)) }
	view.OnOpenViewer = func(b *markdown.SequenceBlock) { send(fmt.Sprintf("viewer %d", b.StartLine)) }
	view.OnRenderMermaid = func(*markdown.MermaidOtherBlock) { send("render") }

	var ops op.Ops
	ready := false
	for {
		switch e := w.Event().(type) {
		case app.DestroyEvent:
			if e.Err != nil {
				send("error " + e.Err.Error())
			}
			return
		case app.FrameEvent:
			changed := false
			if s := in.take(); s != nil {
				view.SetDocument(markdown.Parse([]byte(*s), cache))
				changed = true
			}
			gtx := app.NewContext(&ops, e)
			paint.FillShape(gtx.Ops, mdTh.Background, clip.Rect(image.Rectangle{Max: gtx.Constraints.Max}).Op())
			view.Layout(gtx, th, mdTh)
			e.Frame(gtx.Ops)
			if !ready {
				ready = true
				send("ready")
			} else if changed {
				send("drawn")
			}
		}
	}
}
