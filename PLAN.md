# dyamark — plan

A small, fast Markdown viewer and editor for Omarchy, in the spirit of
omawrite, built around one thing omawrite doesn't do: **drawing Mermaid
sequence diagrams, natively and faithfully.**

## Goals

1. Open a `.md` file and see it rendered, sequence diagrams included,
   near-instantly.
2. Sequence diagrams that **match Mermaid's layout**: same positions,
   spacing, shapes and colours, within a pixel or two. Checked by tests
   against Mermaid itself, not by eye.
3. Make big sequence diagrams easy to read: zoom, pan, fit, a pinned
   participant row, and focus-a-participant.
4. Simple editing: flip into the source, type, save.
5. Lightweight: one Go binary, no browser engine, no JavaScript runtime.
6. Feel like part of Omarchy: keyboard first, follows the theme live,
   shows up in the launcher.

## Non-goals

- Pixel-identical output. Text measurement and anti-aliasing differ from a
  browser, so a box may land a pixel or two off.
- Native rendering of other Mermaid types (flowchart, class, state, …).
  They show as source, with an on-demand render through `mmdc` (see below).
- Mermaid's hand-drawn look, click menus on actors, and full HTML in labels.
- A rich WYSIWYG editor, plugins, tabs, a file tree, or sync.
- Windows or macOS.

## What omawrite is, for reference

C++17 + Qt Quick, about 4,900 lines, about 1 MB installed. It is a
**styled-source editor**: one plain-text box with Markdown syntax coloured in
place, and no rendered preview. Markdown is only rendered for printing and
copy-as-HTML. dyamark borrows its feel and conventions: keyboard first,
portal file picker, atomic and auto save, external-change prompts, and text
size following the desktop. It adds a rendered view, because diagrams need
one.

## Stack

| Piece | Choice | Why |
|---|---|---|
| Language | Go (1.27, set up through mise) | One binary, simple concurrency, easy tests |
| UI | [Gio](https://gioui.org) | Pure-Go, GPU-drawn, Wayland-native, immediate mode, no browser engine |
| Markdown | [`yuin/goldmark`](https://github.com/yuin/goldmark) + GFM | Fast and CommonMark-correct. Start from `gioui.org/x/markdown` + `x/richtext` and extend them for tables and code blocks |
| Sequence diagrams | Our own `seq` package: parser, layout, Gio drawer, SVG writer | The core of the project. A port of Mermaid's sequence code (MIT) |
| Other Mermaid types | `mmdc` on request, shown as a PNG | Starts Chromium only when asked, only for that diagram |
| File watching | [`fsnotify/fsnotify`](https://github.com/fsnotify/fsnotify) | Live reload and external-change prompts |
| File picker | xdg-desktop-portal over D-Bus ([`godbus/dbus`](https://github.com/godbus/dbus)) | Same as omawrite, never a home-made dialog |
| Theme | `~/.local/state/omarchy/current/theme/colors.toml` | Same source as the other Omarchy apps |

### Budget (check in phase 0)

- Binary: about 8–12 MB stripped, fonts included. Measure it.
- Cold start to first paint: under 150 ms.
- Memory: about 40–80 MB RSS for a document with several diagrams.
- Re-layout of a 200-message sequence diagram while typing: under 5 ms.

## Architecture

```text
 file.md ─► goldmark AST ─► view blocks ─┬─► text, headings, lists, tables, code ─► Gio widgets
    ▲                                   ├─► ```mermaid sequenceDiagram ─► seq.Parse ─► seq.Layout ─► seq.Draw (Gio)
    │                                   └─► ```mermaid (other types) ─► source + "render with mmdc"
 fsnotify ─ reload / conflict prompt
 colors.toml ─ fsnotify ─► theme ─► widgets + seq.Theme
```

### The `seq` package (the heart)

Four pieces with clean edges, so each can be tested on its own:

1. **`seq/parse`**: text to AST. A hand-written lexer and parser following
   Mermaid's `sequenceDiagram.jison` grammar and the semantics of
   `sequenceDb.ts`. Errors carry line and column.
2. **`seq/layout`**: AST to geometry (rectangles, lines, text runs, all in
   float32 coordinates). A port of `sequenceRenderer.ts`, with the same
   config defaults (`actorMargin` 50, actor 150×65, `boxMargin` 10,
   `messageMargin` 35, `noteMargin` 10, `boxTextMargin` 5, …). Text is
   measured through a `Measurer` interface, so layout doesn't depend on Gio.
3. **`seq/draw`**: geometry to Gio ops. Arrowheads, crosses, open arrows,
   curved self-messages, dashed lines (split into segments by hand, since Gio
   strokes have no dash style), activation bars, frames with labels.
4. **`seq/svg`**: geometry to SVG. Needed for the parity tests below, and
   it makes export almost free later.

Port with attribution: Mermaid is MIT-licensed. Keep its LICENSE in
`seq/LICENSE.mermaid` and note which upstream version each file follows.

### Parity testing against Mermaid

`mmdc` is installed, so parity is a test, not an opinion:

- `seq/testdata/*.mmd` holds the diagrams. Port the cases from Mermaid's own
  sequence specs, then add our own.
- `bin/golden` runs `mmdc` on each one, using the **same font** as dyamark
  through a Mermaid config, and saves the reference SVG beside it. The
  reference SVGs are committed, so tests don't need `mmdc`.
- `go test ./seq/...` parses the reference SVG and compares the geometry:
  actor boxes, lifelines, message lines and their labels, notes, activation
  bars, and frames. Tolerance is ±2 px, and any failure reports exactly which
  element moved.
- `bin/golden --diff NAME` writes both renders side by side as PNG, for
  looking at a failure.

### Sequence feature coverage (Mermaid 11)

v1:

- `participant` / `actor`, aliases (`participant A as Alice`), implicit
  participants
- All arrow types: `->`, `-->`, `->>`, `-->>`, `-x`, `--x`, `-)`, `--)`,
  and bidirectional `<<->>`, `<<-->>`
- Activations: `+` / `-` on messages, and `activate` / `deactivate`,
  stacked
- Notes: `left of`, `right of`, `over A`, `over A,B`
- Blocks, nested: `loop`, `alt`/`else`, `opt`, `par`/`and`,
  `critical`/`option`, `break`, `rect` with a colour
- `autonumber` (with start and step, and `off`)
- `box` grouping with colour and label
- `create` / `destroy`
- `title`, `%%` comments, `<br>` line breaks, `#59;`-style entity codes
- `wrap` (config and per-message `wrap:`)
- Config from frontmatter and `%%{init: …}%%`: `theme`, `themeVariables`,
  `mirrorActors`, the margins, `showSequenceNumbers`

Later:

- The v11 participant shapes (`@{ "type": "database" }`, `queue`,
  `boundary`, `control`, `entity`, `collections`)
- Actor `links` / `link` menus (shown as a tooltip, not a menu)
- `accTitle` / `accDescr`, used as accessible names

## Views and editing

- **View:** rendered Markdown in a readable column. Sequence diagrams are
  drawn inline, scaled to the column. A parse error shows Mermaid-style in
  place, with the line in the `.md` file, and the last good drawing stays
  visible underneath, dimmed.
- **Edit:** `Ctrl+E` flips to the source in a Gio `widget.Editor`, monospace,
  same column. Plain text in v1, with syntax colouring later.
- **Split:** `Ctrl+\` puts source on the left and the live view on the right,
  re-laid out after an 80 ms debounce. Only the diagram that changed is
  parsed and laid out again; the rest are cached by source hash.
- **Diagram viewer:** `Enter` on a focused diagram, or a click, opens it
  fullscreen. Scroll zooms at the pointer, drag pans, `0` fits, `1` is
  100 %, `W` / `H` fit width / height, `[` / `]` go to the previous / next
  diagram, `N` toggles sequence numbers, `E` jumps into the source at that
  diagram, and `Esc` closes.
- **Sticky participants:** in a tall diagram the actor row stays pinned at
  the top of the viewer while scrolling. It's cheap natively: draw the top
  actors again in a fixed layer.
- **Focus a participant:** click an actor to dim every message, note and
  activation that doesn't involve it.
- **Other Mermaid types:** shown as a code block with a "`R` to render with
  mmdc" hint. The result is cached as a PNG under
  `~/.cache/dyamark/` by source hash. If `mmdc` is missing, say so in the
  window.

### Files

- Open with an argument, `Ctrl+O` (portal), or drag and drop.
- Save is atomic: write `file.md.dyamark-tmp` beside the file, `fsync`,
  rename, and keep the file mode. Like omawrite, auto-save a moment after
  typing stops once the document has a file; untitled drafts are never
  auto-saved.
- On an external change: reload silently if there are no local edits,
  otherwise ask which version to keep, and pause auto-save until then.
- Ask before quitting with unsaved work.

### Theme

- Read `background`, `foreground`, `accent` and `color0`–`color15` from
  `colors.toml`. Watch the directory and re-arm the watcher after each theme
  switch, because the theme's files are replaced.
- Map them to the view palette and to a `seq.Theme` in the same way
  Mermaid's `base` theme derives actor fill, borders, lines, notes and frame
  colours from `themeVariables`.
- A diagram's own `%%{init}%%` theme wins over the Omarchy theme.
- Fall back to a dark palette with the `#FFD60A` accent when Omarchy isn't
  there.
- Corner radius from `hyprctl getoption decoration:rounding -j`.

### Keyboard

| Key | Action |
|---|---|
| `Ctrl+O` | Open (portal) |
| `Ctrl+S` / `Ctrl+Shift+S` | Save / save as |
| `Ctrl+E` | Toggle view / edit |
| `Ctrl+\` | Toggle split |
| `Tab` / `Shift+Tab` | Move between diagrams (view) |
| `Enter` / click | Open diagram viewer |
| `+` `-` `0` `1` `W` `H` | Zoom, fit, 100 %, fit width / height (viewer) |
| `[` `]` | Previous / next diagram (viewer) |
| `N` | Toggle sequence numbers |
| `E` | Edit this diagram's source |
| `R` | Render a non-sequence diagram with mmdc |
| `Ctrl+=` / `Ctrl+-` / `Ctrl+0` | Text size |
| `Esc` | Close viewer / overlay |
| `?` / `F1` | Show all shortcuts |
| `Ctrl+Q` | Quit (asks if unsaved) |

## Project layout

```text
dyamark/
  main.go                    flags, starts the window
  app/                       window, modes, keys, file state, theme wiring
    app.go  view.go  editor.go  viewer.go  files.go  theme.go  portal.go
  markdown/                  goldmark AST → view blocks (Gio widgets)
  seq/
    parse/  layout/  draw/  svg/
    testdata/*.mmd  testdata/*.ref.svg
    LICENSE.mermaid
  fonts/                     bundled UI and diagram fonts, with their licences
  bin/build                  go build -trimpath -ldflags "-s -w -X gioui.org/app.ID=dyamark" -o build/dyamark
  bin/test                   go test ./...
  bin/golden                 regenerate reference SVGs with mmdc
  bin/install                makepkg -fsi in pkgbuild/
  pkgbuild/PKGBUILD  pkgbuild/dyamark.desktop  pkgbuild/dyamark.svg
  README.md  LICENSE  .gitignore
```

## Phases

0. **Spike (a day).** A Gio window on Hyprland, with the app ID matching
   `dyamark.desktop`, Omarchy theme colours, a bundled font, and one
   hard-coded sequence diagram drawn by hand. Measure binary size,
   start-up and RSS against the budget. Confirm the build packages Gio needs
   are present (wayland, libxkbcommon, mesa/EGL, vulkan-headers).
1. **Parser.** `seq/parse` for the v1 syntax, with tests ported from
   Mermaid's specs.
2. **Layout + SVG + parity.** `seq/layout`, `seq/svg`, `bin/golden`, and
   parity tests passing on 30 or more diagrams.
3. **Drawing + viewer.** `seq/draw`, the fullscreen viewer with zoom, pan
   and fit, and the theme mapping.
4. **Markdown view.** goldmark to Gio for the whole document, with sequence
   diagrams inline, error display, other Mermaid types as source, and live
   reload.
5. **Editing.** Editor, split view with per-diagram caching, atomic save and
   auto-save, external-change prompts, portal open/save.
6. **Sequence extras.** Pinned participant row, focus-a-participant, and
   the sequence-numbers toggle.
7. **Packaging.** PKGBUILD, `.desktop`, icon, `?` overlay, and a README with
   a screenshot made from sample content.
8. **Later.** `mmdc` rendering for other types, the v11 participant shapes,
   editor syntax colouring, and a headless `check` / `export` CLI (cheap,
   since `seq/svg` already exists).

Phases 0–5 give a usable v1. Phase 2 is where most of the risk sits, so it
comes early.

## Risks

- **Parity drift.** Mermaid keeps changing its sequence renderer. Pin the
  upstream version the port follows, and re-run `bin/golden` against new
  `mmdc` releases on purpose, not automatically.
- **Text measurement.** Box widths follow label widths, so font differences
  move things. Bundle one font, use it in both dyamark and the golden
  references, and test long and non-Latin labels.
- **Gio before 1.0.** Its API still changes. Pin the version and wrap the
  few places that touch it most (text shaping, the editor, pointer events).
- **Markdown in Gio.** `x/markdown` and `x/richtext` are basic. Tables and
  code blocks will likely need our own widgets.
- **GPU.** Gio draws on the GPU (Vulkan or OpenGL ES). It's fine on any
  normal Omarchy machine, but check the software fallback once.

## Open questions

- Is a plain editor enough for v1, or is Markdown syntax colouring in the
  editor wanted early?
- Should this stay a personal tool, or follow Omarchy's publishing route
  later?
