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

Not yet: layout, drawing from the parser, the Markdown view, editing.
