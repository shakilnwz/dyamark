# dyamark — plan

A small, fast Markdown viewer and editor for Omarchy, in the spirit of
omawrite, built around one thing omawrite doesn't do: **rendering Mermaid
diagrams well, sequence diagrams above all.**

## Goals

1. Open a `.md` file and see it rendered, with every Mermaid block drawn,
   in well under a second.
2. Make sequence diagrams easy to read, even long or wide ones: zoom, pan,
   fit, a participant header that stays visible, and readable theming.
3. Simple editing: flip into the source, type, save. No toolbars.
4. Lightweight: one small binary, the system's webview, no Node runtime,
   no framework.
5. Feel like part of Omarchy: keyboard first, follows the theme live,
   sharp or rounded corners to match Hyprland, shows up in the launcher.

## Non-goals

- A rich WYSIWYG editor, plugins, tabs, a file tree, or sync.
- A native Mermaid renderer. Mermaid's layout lives in mermaid.js, and
  rewriting it (even only for sequence diagrams) costs more than it's worth.
- Windows or macOS.

## Stack

| Piece | Choice | Why |
|---|---|---|
| Language | Go (1.27, already set up through mise) | Simple concurrency for file watching, easy CLI, one static-ish binary |
| Window + rendering | [`webview/webview_go`](https://github.com/webview/webview_go) over **webkit2gtk-4.1** | Uses the WebKit already on the system, so nothing heavy is bundled. Lighter than Wails (no Node build step) and much lighter than Qt WebEngine or Electron (no bundled Chromium) |
| Markdown → HTML | [`yuin/goldmark`](https://github.com/yuin/goldmark) + GFM extension | Fast, CommonMark-correct, runs in Go so the webview only has to show HTML |
| Diagrams | `mermaid.min.js`, pinned 11.x, vendored and embedded with `go:embed` | Works offline and supports every diagram type, with no CDN |
| Editor | Plain `<textarea>` in v1, with CodeMirror 6 as an optional later upgrade | A textarea costs nothing. CodeMirror only comes in if syntax colouring turns out to matter |
| File watching | [`fsnotify/fsnotify`](https://github.com/fsnotify/fsnotify) | Live reload when another program (an agent, git, a script) changes the file |
| Theme | Read `~/.local/state/omarchy/current/theme/colors.toml` | Same source omawrite and the other Omarchy apps use |

The frontend is hand-written HTML, CSS, and JS. It has no bundler,
`package.json`, or React.

### Lightweight budget

- Binary under 15 MB. Mermaid is about 3 MB of it, and the rest is Go and
  the page assets.
- Cold start to first paint under 300 ms for a document without diagrams.
- Mermaid is **loaded only when the document contains a `mermaid` block**.
  A plain Markdown file never pays for it.
- Memory is whatever WebKit's baseline is (roughly 80–150 MB RSS for the
  web process). That's the floor for drawing Mermaid at all.

## How it works

```text
                ┌──────────── Go ────────────┐
 file.md ──► read ─► goldmark ─► HTML ─────────► webview (WebKitGTK)
   ▲            ▲                             │      │
   │ fsnotify ──┘                             │      ├─ mermaid.run() on .mermaid blocks
   │                                          │      ├─ diagram viewer (zoom/pan/fit)
   └── atomic save ◄── Bind("save", text) ◄───┘      └─ editor (textarea)
 colors.toml ─► fsnotify ─► Eval("setTheme({...})")
```

- **Go owns** files, parsing, saving, theme, watching, and the CLI. Go
  functions are exposed to the page with `webview.Bind` (`load`, `save`,
  `render`, `openExternal`, `quit`).
- **The page owns** layout, keys, the editor, and drawing diagrams.
- Goldmark renders ` ```mermaid ` fences as
  `<div class="mermaid" data-src-line="N">…source…</div>`. The line number
  is used for error messages and to jump into the editor at that diagram.
- On every render the page swaps the article HTML, then calls
  `mermaid.render()` per block. It caches rendered SVGs by the hash of the
  diagram source, so typing outside a diagram never redraws it.

## Features

### Viewing (v1, the core)

- Rendered GitHub Flavored Markdown: tables, task lists, strikethrough,
  autolinks, fenced code. Code blocks are monospace, with no syntax
  highlighting in v1.
- A readable column width with typography close to omawrite's, coloured
  from the Omarchy theme.
- Mermaid blocks render inline, centred and scaled to fit the column.
- **A syntax error is shown in place of the diagram** with Mermaid's
  message and the line in the `.md` file, and the last good render stays
  visible underneath, dimmed.
- Live reload when the file changes on disk, keeping the scroll position.
- Links open in the default browser (`xdg-open`), never inside the app.

### Sequence diagrams (v1–v2, the focus)

- **Diagram viewer:** click a diagram, or press `Enter` while it's focused,
  to open it fullscreen inside the window.
  - Scroll to zoom at the pointer, drag to pan, `0` to fit, `1` for 100 %,
    `W` to fit width, `H` to fit height, `Esc` to close.
  - `[` / `]` step to the previous or next diagram in the document.
  - Zoom and pan are written by hand on the SVG `viewBox`, about 100 lines.
    No svg-pan-zoom dependency.
- **Sticky participants (v2):** in a tall sequence diagram, the top row of
  actor boxes stays pinned while scrolling, so you always know which
  lifeline is which. Done by cloning the actor `<g>` nodes into a pinned
  overlay that shares the viewer's transform.
- **Sequence-friendly defaults:** `mirrorActors: true`, `wrap: true`,
  sensible `actorMargin`, `messageMargin` and `noteMargin`, and
  `showSequenceNumbers` toggled with `N` in the viewer.
- **Focus a participant (v2):** click an actor to dim every message that
  doesn't involve it. This is useful in big diagrams and costs only a CSS
  class on matching message paths.
- All other Mermaid types (flowchart, class, state, ER, gantt, mindmap,
  timeline, gitgraph, …) render and work in the viewer too. They just don't
  get the sequence-specific extras.

### Theme

- Read `background`, `foreground`, `accent` and the `color0`–`color15`
  palette from `colors.toml`, and watch the directory, re-arming after each
  theme switch because the theme files are replaced.
- Map them to CSS variables for the page and to Mermaid `themeVariables`
  under the `base` theme (actor fill, borders, line colour, note colours,
  signal text). Then re-render diagrams.
- Fall back to a dark palette with the `#FFD60A` accent when Omarchy's
  theme isn't present.
- Corner radius from `hyprctl getoption decoration:rounding -j`, as a CSS
  variable.

### Editing (v1, simple)

- `Ctrl+E` toggles between **view** and **edit**, like omawrite. Edit is a
  full-window monospace textarea at the same column width.
- `Ctrl+\` toggles **split** (source left, live preview right, re-rendered
  after a 150 ms debounce). Split is handy when writing a sequence diagram.
- Opening the editor from a diagram (`E` in the viewer) puts the cursor on
  that diagram's first line.
- `Ctrl+S` saves atomically: write `file.md.dyamark-tmp` beside it, `fsync`,
  then rename. File mode is kept.
- An unsaved-changes marker in the title. `Ctrl+Q` and closing the window
  ask before discarding work.
- If the file changes on disk while there are unsaved edits, ask which
  version to keep instead of overwriting either.
- `Ctrl+Z` / `Ctrl+Shift+Z` use the textarea's native undo.

### Keyboard

| Key | Action |
|---|---|
| `Ctrl+O` | Open file (xdg-desktop-portal file chooser) |
| `Ctrl+S` | Save |
| `Ctrl+E` | Toggle view / edit |
| `Ctrl+\` | Toggle split |
| `Enter` / click | Open the focused diagram in the viewer |
| `Tab` / `Shift+Tab` | Move focus between diagrams (view mode) |
| `+` `-` `0` `1` `W` `H` | Zoom in, zoom out, fit, 100 %, fit width, fit height (viewer) |
| `[` `]` | Previous / next diagram (viewer) |
| `N` | Toggle sequence numbers (viewer) |
| `E` | Edit this diagram's source |
| `Esc` | Close the viewer |
| `?` | Show all shortcuts |
| `Q` / `Ctrl+Q` | Quit (asks if unsaved) |

## CLI

```text
dyamark [FILE]                  open the GUI (empty document without FILE)
dyamark check FILE [--json]     parse the Markdown and every Mermaid block; exit 1 on errors
dyamark export FILE [--out DIR] [--format svg|png] [--json]
                                write each diagram as diagram-N.svg / .png
dyamark --version | --help
```

`check` and `export` need mermaid.js, so they run it in a hidden webview
window. If that turns out to need a display, they fall back to `mmdc` when
it's installed and say clearly when neither works. These commands are for
agents and scripts. They're phase 3 and not needed for v1.

## Project layout

```text
dyamark/
  main.go               flags, subcommands, starts the window
  app.go                the App: current file, dirty state, Bind'ed methods
  markdown.go           goldmark setup, mermaid fence → div with line numbers
  files.go              load, atomic save, fsnotify watcher
  theme.go              colors.toml + hyprctl rounding, watcher, → JSON for the page
  portal.go             xdg-desktop-portal FileChooser over D-Bus (godbus)
  cli.go                check / export
  web/
    index.html
    app.css             layout, typography, theme variables
    app.js              modes, keys, render loop, SVG cache
    viewer.js           fullscreen diagram viewer: zoom, pan, fit, sticky actors
    mermaid.min.js      vendored, pinned, with its LICENSE
  *_test.go             markdown, atomic save, theme parsing, watcher
  bin/build             go build -trimpath -ldflags "-s -w" -o build/dyamark
  bin/test              go test ./...
  bin/install           makepkg -fsi in pkgbuild/
  pkgbuild/PKGBUILD     depends: webkit2gtk-4.1 gtk3 xdg-desktop-portal
  pkgbuild/dyamark.desktop
  pkgbuild/dyamark.svg
  README.md  LICENSE  .gitignore
```

Set the Wayland app id with `g_set_prgname("dyamark")` before GTK starts,
through a few lines of cgo, so Hyprland and the launcher match the window
to `dyamark.desktop` and its icon.

## Phases

1. **Skeleton.** Window opens a file, goldmark renders it, theme applied,
   live reload on file change. `bin/build`, `bin/test`.
2. **Mermaid inline.** Lazy-load mermaid, render blocks, SVG cache, inline
   errors with line numbers, theme mapped to `themeVariables`.
3. **Diagram viewer.** Fullscreen overlay, zoom/pan/fit, next/previous,
   sequence numbers toggle.
4. **Editing.** View/edit toggle, split view, atomic save, unsaved-changes
   prompts, external-change conflict prompt, portal open dialog.
5. **Sequence extras.** Sticky participant header, focus-a-participant.
6. **Packaging.** PKGBUILD, `.desktop`, icon, `?` overlay, README with a
   screenshot made from sample content.
7. **CLI.** `check` and `export`, with `--json`.

Phases 1–4 make a usable v1. Phase 5 is what makes it better than other
viewers for sequence diagrams.

## Risks

- **WebKitGTK vs Chromium.** Mermaid is developed against Chromium. WebKit
  renders it fine in practice, but text measurement can differ slightly. Test
  a set of sample sequence diagrams early, in phase 2.
- **cgo.** `webview_go` needs cgo and the webkit2gtk headers, which the
  system already has. Builds aren't fully static, which is fine for an Arch
  package.
- **Headless export.** WebKitGTK may refuse to start without a display. The
  `mmdc` fallback covers that, and it's phase 7 anyway.
- **Huge diagrams.** Re-rendering hundreds of messages on every keystroke
  in split mode could stutter. The debounce and per-block SVG cache should
  cover it. If not, render only the block under the cursor while typing.

## Open questions

- Is a plain textarea enough for editing, or is syntax colouring wanted in
  v1, which would mean CodeMirror at about 150 KB?
- Should the repo follow Omarchy's publishing route later (omacom
  packages), or stay a personal tool?
