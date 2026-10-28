# dyamark

A small, fast Markdown viewer and editor for Omarchy, built around drawing **Mermaid sequence diagrams natively and faithfully**.

It's early: see [Status](#status) for what works today and [PLAN.md](PLAN.md) for the rest.

## Building & Testing

```bash
# Build the stripped binary (build/dyamark)
./bin/build

# gofmt check, go vet, and the tests
./bin/test

# Print start-up time and memory after the first frame, then quit
./build/dyamark --measure --exit-after

# Build and install the Arch package
./bin/install
```

Gio is built for Wayland and OpenGL ES only, through the `novulkan,nox11`
build tags set in `bin/env`. Running `go` by hand needs the same tags, for
example `GOFLAGS=-tags=novulkan,nox11 go test ./...`, or Gio's Vulkan backend
asks for `vulkan/vulkan.h`.

## Releases

Pushing a tag like `v0.2.0` runs `.github/workflows/release.yml`: it runs
the tests and publishes a GitHub release with an Arch `.pkg.tar.zst`, a
`.deb` and an `.rpm` (made by nfpm from `pkgbuild/nfpm.yaml`), and a
Windows `.exe`. The Linux packages need glibc 2.39 or newer. On Windows
there's no file dialog yet (it uses xdg-desktop-portal), so files are
opened from the command line or "Open with", and an untitled draft can't
be saved.

## Status

**Phase 0 (spike):** done.

- A Gio window on Hyprland (`app_id=dyamark`) drawing a hard-coded sequence
  diagram, in dp, so it keeps its proportions on any output scale.
- Omarchy theme colours, followed live through theme switches, and
  Hyprland's corner rounding.
- Bundled Liberation Sans. System fonts, for scripts it doesn't cover, load
  in the background after the first frame.
- Measured on an AMD Raven laptop: 8.4 MB binary, 88–111 ms to first paint,
  ~105 MB RSS, of which ~46 MB is Mesa's shared LLVM and gallium libraries
  (PSS ~62 MB).

**Phase 1 (parser):** done. `seq/parse` reads Mermaid 12 sequence diagrams:
everything in the plan's v1 list, plus typed participants, half arrows,
central connections, `par_over`, links and properties. Every item carries
its source line. Its accept/reject decisions are checked against `mmdc`.

**Phase 2 (layout, SVG, parity):** done. `seq/layout` is a port of
Mermaid's sequence renderer; on all 39 diagrams in `seq/testdata` every
box, line and label is within 2 px of what `mmdc` draws.

**Phase 3 (drawing, viewer):** done. Diagrams are drawn natively with Gio.
The fullscreen viewer zooms at the pointer, pans, fits, keeps the
participant row pinned while scrolling, and toggles sequence numbers.

**Phase 4 (Markdown view):** done. The whole document is drawn, with
sequence diagrams inline, parse errors over the last good drawing, other
Mermaid types as source with `R` to render them through `mmdc`, and live
reload when the file changes.

**Phase 5 (editing):** done.

- `Ctrl+E` shows the source, `Ctrl+\` puts it beside the rendered view,
  which follows the typing 80 ms after it stops. Only the diagrams that
  changed are laid out again.
- Saves are atomic, and a document with a file saves itself a second after
  typing stops. Untitled drafts are only saved when asked.
- A change on disk is taken in quietly when there are no edits here, and
  otherwise asks which version to keep.
- `Ctrl+O` and `Ctrl+Shift+S` use the desktop's file dialogs
  (xdg-desktop-portal). Closing the window with an unsaved draft asks first.
- `E` in the viewer jumps to that diagram's source.

**Phase 6 (sequence extras):** done. In the viewer, clicking a
participant dims every message, note and activation it isn't part of;
clicking it again or `Esc` shows everything. The pinned participant row
works with it.

Memory is over the plan's budget: about 105–112 MB RSS, mostly Mesa's
libraries; Go's own heap is about 10 MB.

Not yet: phases 7 and 8 in [PLAN.md](PLAN.md).

## Keys

| Key | Action |
|---|---|
| `Ctrl+O` | Open |
| `Ctrl+S` / `Ctrl+Shift+S` | Save / save as |
| `Ctrl+E` | Source / view |
| `Ctrl+\` | Source and view side by side |
| `Tab` / `Shift+Tab` | Move between diagrams |
| `Enter` / click | Open the diagram viewer |
| `+` `-` `0` `1` `W` `H` | Zoom, fit, 100 %, fit width / height (viewer) |
| `[` `]` | Previous / next diagram (viewer) |
| `N` | Toggle sequence numbers (viewer) |
| `E` | Edit this diagram's source (viewer) |
| Click a participant | Show only what it takes part in (viewer) |
| `R` | Render another Mermaid diagram type with `mmdc` |
| `Ctrl+=` / `Ctrl+-` / `Ctrl+0` | Text size |
| `Esc` | Close the viewer, or cancel a question |
| `Ctrl+Q` | Quit |
