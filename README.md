<p align="center">
  <img src="pkgbuild/dyamark.svg" width="96" height="96" alt="dyamark icon">
</p>

<h1 align="center">dyamark</h1>

<p align="center">
  A fast, native Markdown viewer and editor that draws Mermaid sequence diagrams faithfully, without a browser engine.
</p>

<p align="center">
  <a href="https://github.com/shakilnwz/dyamark/releases/latest"><img alt="Latest release" src="https://img.shields.io/github/v/release/shakilnwz/dyamark"></a>
  <a href="LICENSE"><img alt="MIT licence" src="https://img.shields.io/github/license/shakilnwz/dyamark"></a>
  <img alt="Go 1.27" src="https://img.shields.io/badge/go-1.27-00ADD8">
</p>

| Light | Dark |
|---|---|
| ![dyamark preview, light scheme](docs/screenshots/preview-light.png) | ![dyamark preview, dark scheme](docs/screenshots/preview-dark.png) |

<p align="center"><sub>dyamark's view in PhpStorm's Markdown preview. The desktop app draws the same view with the same code.</sub></p>

## What is dyamark?

dyamark opens a Markdown file and shows it rendered, with Mermaid
sequence diagrams drawn inline. It is one Go binary, drawn on the GPU with
[Gio](https://gioui.org): no Electron, no web view, no JavaScript. It
starts in about 100 ms.

The sequence diagrams are the point. dyamark doesn't approximate them: its
layout engine is a port of Mermaid's own sequence renderer, and a test
suite compares every box, line and label against what Mermaid's `mmdc`
draws. On all 39 reference diagrams the two agree to within 2 px.

It was made for [Omarchy](https://omarchy.org) and follows its theme
live, but should run on any Wayland desktop. The same renderer, compiled
to WebAssembly, is also a Markdown preview plugin for PhpStorm and other
JetBrains IDEs.

## Features

**Reading**
- GitHub-flavoured Markdown: headings, lists, task lists, tables, code
  blocks, quotes, links and emphasis.
- Mermaid sequence diagrams drawn natively and inline, scaled to the
  column.
- A diagram with a syntax error shows the error and the line it's on, over
  the last good drawing.
- Other Mermaid diagram types show as source; `R` renders them through
  `mmdc`, if it's installed.
- Links open in the desktop's app for them; links to Markdown files open
  in dyamark.
- Live reload when the file changes on disk.

**Diagram viewer**
- Fullscreen, with zoom at the pointer, pan, fit, fit width or height,
  and 100 %.
- The participant row stays pinned at the top while you scroll a tall
  diagram.
- Click a participant to dim everything it doesn't take part in.
- Toggle sequence numbers, and jump to a diagram's source with `E`.

**Editing**
- Source view (`Ctrl+E`) and source beside the rendered view (`Ctrl+\`),
  which follows your typing and scrolls to the block you're in.
- Atomic saves, auto-save a second after typing stops, and a prompt when
  the file changes on disk while you have edits.
- Find, text size, word count, and caret position.

**Desktop**
- Follows the Omarchy theme and Hyprland's corner rounding live.
- Keyboard first: `?` or `F1` lists every key.
- Native file dialogs through xdg-desktop-portal, and drag and drop.

## Installation

### Linux packages

Every [release](https://github.com/shakilnwz/dyamark/releases/latest) has
packages for x86-64 Linux. They need glibc 2.39 or newer (Arch, Fedora 40+,
Debian 13, Ubuntu 24.04 or newer) and a Wayland session.

```bash
# Arch Linux / Omarchy
sudo pacman -U dyamark-<version>-1-x86_64.pkg.tar.zst

# Debian / Ubuntu
sudo apt install ./dyamark_<version>-1_amd64.deb

# Fedora
sudo dnf install ./dyamark-<version>-1.x86_64.rpm
```

The package installs `/usr/bin/dyamark`, a launcher entry and an icon.

### Building from source

You need [Go](https://go.dev/dl/) 1.27 or newer and the Wayland,
xkbcommon and EGL/GLES development files.

```bash
# Arch Linux
sudo pacman -S --needed go git wayland libxkbcommon libglvnd

# Debian / Ubuntu (install Go itself from go.dev)
sudo apt install git pkg-config libwayland-dev libxkbcommon-dev libegl1-mesa-dev libgles2-mesa-dev
```

Then:

```bash
git clone https://github.com/shakilnwz/dyamark.git
cd dyamark

./bin/build      # builds build/dyamark
./bin/install    # Arch only: builds and installs the package with makepkg
```

### Windows (experimental)

Releases include a Windows `.exe`. It has no file dialog yet, so open
files from the command line or with "Open with", and untitled drafts
can't be saved.

### Optional: `mmdc`

To render Mermaid diagram types other than sequence diagrams, install
[Mermaid CLI](https://github.com/mermaid-js/mermaid-cli):

```bash
npm install -g @mermaid-js/mermaid-cli
```

dyamark runs it only when you ask: `R` renders the diagram under the
pointer, or else the first unrendered one on screen. Results are cached
under `~/.cache/dyamark/`.

## Usage

```bash
dyamark notes.md     # open a file
dyamark              # open with a sample document
```

You can also open files from the app launcher, with `Ctrl+O`, or by
dropping them on the window.

### Reading and editing

- Scroll with the arrow keys, `PgUp` / `PgDn`, `Space`, `Home` and `End`.
- `Tab` and `Shift+Tab` move between diagrams; `Enter` or a click opens
  the focused one in the viewer.
- `Ctrl+E` switches between the rendered view and the source.
  `Ctrl+\` shows both side by side.
- A document with a file saves itself a second after you stop typing.
  `Ctrl+S` saves now; `Ctrl+Shift+S` saves as.
- If the file changes on disk while you have unsaved edits, dyamark asks
  which version to keep.

### The diagram viewer

Open a diagram with `Enter` or a click. Scroll to zoom at the pointer,
drag to pan, and use the keys below to fit. Click a participant to see
only the messages, notes and activations it takes part in; click it again
or press `Esc` to see everything. `E` jumps to the diagram's source in the
editor.

### Theme

On Omarchy, dyamark reads the current theme from
`~/.local/state/omarchy/current/theme/colors.toml` and follows theme
switches live. Corner rounding comes from Hyprland's
`decoration:rounding`. Elsewhere it uses a built-in dark palette. A
diagram's own `%%{init: …}%%` theme wins over both.

### Keyboard shortcuts

| Key | Action |
|---|---|
| `Ctrl+O` | Open |
| `Ctrl+F` | Find; `Enter` / `Shift+Enter` for the next / previous match |
| `Ctrl+S` / `Ctrl+Shift+S` | Save / save as |
| `Ctrl+E` | Source / view |
| `Ctrl+\` | Source and view side by side |
| `↑` `↓` `PgUp` `PgDn` `Space` `Home` `End` | Scroll the view |
| Click a link | Open it |
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
| `?` / `F1` | List the keys |
| `Ctrl+Q` | Quit |

## Mermaid sequence diagram support

dyamark follows Mermaid 12. It draws:

- `participant` and `actor`, aliases, and implicit participants
- every arrow type, including bidirectional and half arrows, and `()`
  central connections
- activations, with `+` / `-` on messages or `activate` / `deactivate`,
  stacked
- notes `left of`, `right of`, and `over` one or two participants
- `loop`, `alt` / `else`, `opt`, `par` / `and`, `critical` / `option`,
  `break` and coloured `rect` blocks, nested
- `autonumber` with a start, step and `off`
- `box` groups, `create` / `destroy`, and `title`
- `<br>` line breaks, entity codes, `wrap`, and `%%` comments
- config from frontmatter and `%%{init: …}%%`: `theme`, `themeVariables`,
  `mirrorActors`, margins and `showSequenceNumbers`

The typed participant shapes (`database`, `queue`, `boundary`, `control`,
`entity`, `collections`) are parsed but drawn as plain boxes for now.

Where Mermaid's grammar and its documentation disagree, dyamark does what
`mmdc` does: for example `#` starts a comment, and a message needs its
`:`.

## JetBrains plugin

dyamark is also a Markdown preview for PhpStorm 2026.2 (build 262). It
should work in other IntelliJ-based IDEs of that build with the bundled
Markdown plugin, but only PhpStorm is tested.

- Sequence diagrams drawn by dyamark, compiled to WebAssembly.
- The diagram viewer: click a diagram, or press `Tab` and `Enter`. `E` in
  the viewer puts the IDE editor's caret on the diagram's first line.
- Colours from the editor's colour scheme, updated live when it changes.
- With the split editor's synchronised scrolling on, the editor and the
  preview scroll each other.
- Web links open in your browser, relative file links in the IDE, and
  `#heading` links scroll the preview.

**Install**

1. Download `dyamark-jetbrains-<version>.zip` from the
   [latest release](https://github.com/shakilnwz/dyamark/releases/latest),
   or build it with `./bin/plugin`.
2. In the IDE, open **Settings › Plugins › ⚙ › Install Plugin from Disk…**
   and choose the zip.
3. Open a Markdown file and click **Use Dyamark** on the notification, or
   choose **dyamark** under **Settings › Languages & Frameworks ›
   Markdown › Preview**.

## Development

```bash
./bin/test                               # gofmt check, go vet, and the tests
./bin/build                              # build/dyamark, stripped
./build/dyamark --measure --exit-after   # print start-up time and memory, then quit
./bin/golden                             # regenerate the reference SVGs with mmdc
./bin/wasm                               # the WebAssembly build, into build/web/
./bin/webcheck                           # check the web build in headless Chromium
./bin/plugin                             # the JetBrains plugin zip, into build/
```

The desktop build uses Gio's Wayland and OpenGL ES backends only, through
the `novulkan,nox11` build tags that `bin/env` sets. Running `go` by hand
needs the same tags, for example:

```bash
GOFLAGS=-tags=novulkan,nox11 go test ./...
```

Without them, Gio's Vulkan backend asks for `vulkan/vulkan.h`.

`bin/webcheck` needs Node 22+ and Chromium. `bin/plugin` needs JDK 25; it
builds against the PhpStorm in `/opt/phpstorm` if there is one, and
otherwise downloads PhpStorm 2026.2.4.

### Parity tests

`seq/testdata/` holds Mermaid sequence diagrams (`*.mmd`) and the SVG
`mmdc` draws for each (`*.ref.svg`). The tests lay out every diagram and
compare the geometry with the reference, failing on any element more than
2 px off and naming it. The references are committed, so the tests don't
need `mmdc`; `bin/golden` regenerates them.

### Project layout

| Path | What's there |
|---|---|
| `main.go`, `app/` | The window, modes, keys, files, theme and toolbar |
| `markdown/` | Markdown to view blocks, with goldmark |
| `seq/parse/` | The Mermaid sequence parser, from mmgo, extended |
| `seq/layout/` | Layout, a port of Mermaid's sequence renderer |
| `seq/draw/`, `seq/svg/` | Drawing to Gio, and to SVG for the parity tests |
| `viewer/` | The fullscreen diagram viewer |
| `palette/` | The page palette and the diagram theme derived from it |
| `cmd/dyamark-web/`, `web/` | The WebAssembly build |
| `jetbrains/` | The JetBrains plugin (Kotlin, Gradle) |
| `fonts/` | Bundled fonts and their licences |
| `pkgbuild/` | PKGBUILD, nfpm config, desktop entry and icon |
| `docs/` | The design plan, status, feature plans and reports |

### Releases

Pushing a tag like `v1.4.0` runs `.github/workflows/release.yml`. It runs
the tests and publishes a GitHub release with the Arch, Debian and RPM
packages (made by nfpm from `pkgbuild/nfpm.yaml`), the Windows `.exe`, and
the JetBrains plugin zip.

## Documentation

[`docs/`](docs/README.md) has the [design plan](docs/plan.md), the
[project status](docs/status.md), plans for single features, and reports
on research and reviews.

## Licence and credits

dyamark is released under the [MIT licence](LICENSE).

It builds on:

- [Mermaid](https://github.com/mermaid-js/mermaid) (MIT): `seq/layout` is
  a port of its sequence renderer. Licence in `seq/LICENSE.mermaid`.
- [mmgo](https://github.com/julianshen/mmgo) (MIT): `seq/parse` started as
  a copy of its sequence parser. Licence in `seq/parse/LICENSE.mmgo`.
- [Liberation fonts](https://github.com/liberationfonts/liberation-fonts)
  (SIL OFL 1.1), bundled; Dyamark Mono is Liberation Mono with one glyph
  added. See [`fonts/README.md`](fonts/README.md).
- [Gio](https://gioui.org), [goldmark](https://github.com/yuin/goldmark),
  [fsnotify](https://github.com/fsnotify/fsnotify) and
  [godbus](https://github.com/godbus/dbus).
