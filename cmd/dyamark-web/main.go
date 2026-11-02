//go:build js && wasm

// Command dyamark-web is dyamark's preview built for WebAssembly, for the
// PhpStorm plugin's preview panel. The preview itself is package web; this
// binds it to the page.
//
// The page calls, on globalThis.dyamark:
//
//	setSource(text)     the whole document, on each change
//	setTheme(json)      the palette, as web.Colors; returns an error's
//	                    text, or null
//	scrollToLine(line)  puts the block holding line, 1-based, at the top
//
// dyamark answers through the page's dyamarkSend(message), one message
// each: "ready" with the first frame, "drawn" with the frame that first
// shows a new source, "link URL", "edit LINE", "scroll LINE", "log TEXT"
// and "error TEXT".
package main

import (
	"fmt"
	"syscall/js"

	"dyamark/fonts"
	"dyamark/web"

	"gioui.org/app"
	"gioui.org/op"
	"gioui.org/text"
)

func main() {
	w := new(app.Window)
	faces, err := fonts.Collection()
	if err != nil {
		send("error fonts: " + err.Error())
	}
	p := web.New(host{}, text.NewShaper(text.NoSystemFonts(), text.WithCollection(faces)), w.Invalidate)

	js.Global().Set("dyamark", js.ValueOf(map[string]any{
		"setSource": js.FuncOf(func(_ js.Value, args []js.Value) any {
			if len(args) > 0 {
				p.SetSource(args[0].String())
			}
			return nil
		}),
		"setTheme": js.FuncOf(func(_ js.Value, args []js.Value) any {
			if len(args) == 0 {
				return "setTheme: no palette"
			}
			if err := p.SetTheme(args[0].String()); err != nil {
				return err.Error()
			}
			return nil
		}),
		"scrollToLine": js.FuncOf(func(_ js.Value, args []js.Value) any {
			if len(args) > 0 {
				p.ScrollToLine(args[0].Int())
			}
			return nil
		}),
		"zoomIn": js.FuncOf(func(_ js.Value, _ []js.Value) any {
			p.ZoomIn()
			return nil
		}),
		"zoomOut": js.FuncOf(func(_ js.Value, _ []js.Value) any {
			p.ZoomOut()
			return nil
		}),
		"resetZoom": js.FuncOf(func(_ js.Value, _ []js.Value) any {
			p.ResetZoom()
			return nil
		}),
		"toggleHelp": js.FuncOf(func(_ js.Value, _ []js.Value) any {
			p.ToggleHelp()
			return nil
		}),
	}))
	go run(w, p)
	app.Main()
}

func run(w *app.Window, p *web.Preview) {
	var ops op.Ops
	for {
		switch e := w.Event().(type) {
		case app.DestroyEvent:
			if e.Err != nil {
				send("error " + e.Err.Error())
			}
			return
		case app.FrameEvent:
			gtx := app.NewContext(&ops, e)
			p.Layout(gtx)
			e.Frame(gtx.Ops)
		}
	}
}

// host passes the preview's messages to the page, which passes them on to
// the plugin.
type host struct{}

func (host) Ready()              { send("ready") }
func (host) Drawn()              { send("drawn") }
func (host) OpenLink(url string) { send("link " + url) }
func (host) EditLine(line int)   { send(fmt.Sprint("edit ", line)) }
func (host) Scrolled(line int)   { send(fmt.Sprint("scroll ", line)) }
func (host) Log(msg string)      { send("log " + msg) }

func send(msg string) {
	if f := js.Global().Get("dyamarkSend"); f.Type() == js.TypeFunction {
		f.Invoke(msg)
	}
}
