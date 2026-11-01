# Plan: a PhpStorm plugin that draws Markdown previews with dyamark

Date: 2026-11-01. Status: step 1 done, except one question on focus
(below). Step 2 is next.

## Goal

Every Markdown preview in PhpStorm is drawn by dyamark: the same view,
the same sequence diagrams, the same diagram viewer, inside the IDE's
own preview pane. PhpStorm keeps doing what it already does well (the
editor, saving, find, VCS, its Vim plugin), and dyamark only draws.

- The split editor's preview half, and "preview only", show dyamark.
- It follows the typing live, and the editor and preview scroll together.
- Links, `E` from the diagram viewer, and the participant focus all
  work as in the app, with the IDE in place of the app's own editor.

## How it fits in PhpStorm

PhpStorm 2026.2's bundled Markdown plugin draws its preview through a
pluggable **preview panel provider** (the
`org.intellij.markdown.html.panel.provider` extension point). It ships
two: the JCEF (Chromium) one, which is the default, and "Compose
(experimental)". Whichever one is picked under *Settings › Languages &
Frameworks › Markdown › Preview* draws every Markdown preview.

A provider also chooses the text its panel is given. The JCEF provider
converts the document to HTML first. The default, which Compose keeps,
hands over the **raw Markdown**: `document.getText()`, checked in the
2026.2 jars. So dyamark registers a third provider, keeps the default,
and its panel's `setHtml(text, …)` receives the Markdown source,
ready for `markdown.Parse`.

The panel can't hold a Gio window: Gio here is built for Wayland only,
and the IDE is Swing. It holds a JCEF browser running **dyamark compiled
to WebAssembly**. Gio has a WebAssembly backend that draws with WebGL,
and `markdown/` and `seq/` already build for it unchanged. The binary
is the same code, so the drawing matches the app.

```text
 PhpStorm editor ─ document ─► Markdown plugin ─► setHtml(raw markdown) ─┐
                                                                        ▼
                                   DyamarkPanel (Kotlin, JCEF browser)
                                     │ executeJavaScript: setSource / setTheme / scrollToLine
                                     ▼
                         dyamark.wasm (Gio, WebGL): markdown.View + viewer
                                     │ JBCefJSQuery: openLink / editLine / scrolled / ready
                                     ▼
                              DyamarkPanel ─► IDE: browser, open file, caret, scroll
```

## Spike (2026-11-01)

A throwaway `main` was built outside the repo, through `go build -overlay`.
It ran `markdown.View` in a Gio window, with a JavaScript function that
replaces the document.

| Check | Result |
|---|---|
| `markdown/`, `seq/`, `fonts/` for `GOOS=js GOARCH=wasm` | Build unchanged |
| `app/` for wasm | Doesn't build: `godbus/dbus` (the portal). `fsnotify`, `xdg-open`, `hyprctl` and `mmdc` also can't work there |
| `.wasm` size, stripped, fonts included | 17.4 MB; 4.9 MB gzipped, which is about its size in the plugin zip |
| Headless Chromium, software WebGL (SwiftShader) | Drew the heading, bold text and a sequence diagram correctly. A document pushed from JavaScript replaced the first one |
| Time to first frame there | About 150 ms, including the wasm load. Headless timing is rough; measure again in PhpStorm |
| `FileEditorPolicy.HIDE_OTHER_EDITORS` | Exists, but isn't needed: the provider route keeps the IDE's own split editor |
| `MarkdownHtmlPanelProvider`, `MarkdownHtmlPanel` | `@ApiStatus.Experimental`: allowed, but may change between releases |
| JCEF API | `JBCefApp.JBCefCustomSchemeHandlerFactory`, `JBCefJSQuery`, `JBCefBrowserBuilder` all there |
| This machine's PhpStorm | 2026.2.4 (build 262), running on `WLToolkit` (native Wayland) |

## Step 1 in PhpStorm (2026-11-01)

A bare plugin (`jetbrains/`) with the provider and a panel, and the
spike's wasm (`cmd/dyamark-web`, built by `bin/wasm`), run in a sandbox
PhpStorm 2026.2.4 with `./gradlew runIde`. With `-Pspike` it opens
`spike.md` in dyamark's preview, screenshots it, types into the document,
screenshots again, opens four more previews, and logs the times.

JCEF here is Chromium 144, out of process (`remote` JCEF, so always
off-screen rendering). Serving the files with a per-browser
`CefRequestHandler` and `CefResourceHandler` works in that mode, as does
`JBCefJSQuery`. The default source text preprocessor did hand the panel
raw Markdown.

| Check | GPU on (default) | GPU off | GPU off, `--enable-unsafe-swiftshader` |
|---|---|---|---|
| WebGL | WebGL 2, ANGLE on the Radeon Vega 11 (radeonsi) | **None: the preview stays blank** | WebGL 2, SwiftShader |
| Drawn correctly | Yes | No | Yes |
| Page start to first frame, IDE idle | 510–530 ms (one reload 1.2 s) | — | 970 ms |
| Of which loading and compiling the wasm | 350–390 ms | — | 440 ms |
| Typing to drawn | 187 ms, of which 158 ms is the Markdown plugin's own wait before `setHtml` | — | 117 ms |
| wasm memory | 39 MB | 28 MB | 36 MB |

- **Memory.** Every preview is its own Chromium renderer process: 125–134
  MB PSS (220–234 MB RSS) each with five open. A plain HTML page
  in the same IDE was 90 MB PSS, so dyamark adds about 35–45 MB per
  preview. The GPU process (139 MB) and `cef_server` (129 MB) are shared,
  and are there anyway once any JCEF page has been opened.
- **The first preview after the IDE starts** was ready 1.8 s after its
  panel was made, on a sandbox that had run before. On the sandbox's first
  runs it took 7–22 s, while JCEF cleared its cache and the IDE indexed and
  loaded its own "What's New" JCEF page. Later previews took about 1 s
  from opening the file to the first frame, four loading at once.
- **Keys** were posted through the IDE's event queue, so they met the
  keymap the way typed keys do, with IdeaVim 2.48.1 and the "XWin copy"
  keymap loaded. `↓` `PgDn` `Space` `Home` `End` `Tab` `Shift+Tab` `j`
  `R` `Ctrl+0` and `Enter` reached dyamark, and it acted on them: `Tab`
  focused the diagram, `Enter` asked for the viewer. **`Ctrl+=` and
  `Ctrl+-` didn't**: the IDE takes them. Step 4 adds IDE actions for
  text size that forward to the focused preview.
- **Focus on click: an open question.** In a manual try in the sandbox,
  clicks on the diagram reached dyamark but no key presses arrived. The
  automated run gave the preview focus itself
  (`IdeFocusManager.requestFocus` and `CefBrowser.setFocus(true)`), so a
  click may not be giving it the keyboard. Step 4 makes a mouse press in
  the preview take the focus the same way, and checks it by hand.
- **Gio doesn't say when WebGL is missing.** With the GPU off it still
  reported frames, and drew nothing. The page's own WebGL check is the
  one to trust: step 4 shows a message there instead of a blank pane.
- **A reload loses the document.** After a page reload (DevTools, or a
  renderer crash) dyamark comes up empty. Step 4 sends the source again
  on every `ready`, not only the first.
- `scrollToMarkdownSrcOffset(offset, smooth)` is deprecated in 2026.2;
  step 6 uses its replacement.
- A sandbox needs the PhpStorm licence key copied to
  `.intellijPlatform/sandbox/…/config_runIde/` (gitignored), or it opens
  on a modal licence dialog.

## Decisions

- **The IDE edits, dyamark draws.** The wasm build has no editor,
  split, save, auto-save, file watching, portal dialogs, toolbar or
  change prompts. PhpStorm has its own.
- **Kept from the app:** the rendered view, its scrolling keys, `Tab` /
  `Enter` between diagrams, the diagram viewer with zoom, pan, fit,
  `N`, `[` `]`, the participant focus and the pinned names, the text
  size keys, and `?` for the keys that work here.
- **`E` in the viewer** moves the IDE's caret to that diagram's source
  and focuses the editor, instead of opening dyamark's editor.
- **Links:** `http(s)` and `mailto` go to the desktop browser. A
  relative `.md` (or any project file) opens in the IDE. `#anchor`
  scrolls the view.
- **Colours come from the IDE's editor colour scheme** (background,
  foreground, link colour for the accent, error colour). They go through
  the same derivation as the Omarchy colours, so diagrams pick up the
  scheme the way they pick up an Omarchy theme. A diagram's own
  `%%{init}%%` theme still wins.
- **Other Mermaid types show as source** in v1, as when `mmdc` is
  missing. Running `mmdc` through the plugin comes later.
- **One repo.** The plugin lives in `jetbrains/` here, so a change to
  `markdown/` or `seq/` and the plugin that ships it move together.
- **Target PhpStorm 2026.2 (build 262) and up**, and depend on the
  bundled Markdown plugin. It should work in any IntelliJ-based IDE
  with that plugin, but only PhpStorm is tested.
- **Opt-in, then default.** Installing it adds "dyamark" to the Preview
  choices. The first Markdown file opened afterwards shows a one-time
  notification offering to make dyamark the preview.

## The Go side

1. **Move the desktop-free parts out of `app/`.** The diagram viewer
   (`app/viewer.go`) imports only Gio and `seq`, but sits in the package
   that imports D-Bus. It uses `app.Theme` and a few colour helpers.
   - `palette/`: `Theme`, the colour helpers and `deriveSeqTheme`.
     `app/theme.go` keeps loading `colors.toml`, the watcher and `hyprctl`.
   - `viewer/`: the `Viewer`, unchanged except for its imports.
   `app/` uses both, and works and tests as before.
2. **`cmd/dyamark-web/`** (`//go:build js && wasm`) is the wasm `main`. It
   is kept thin: the logic is in a `web/` package that builds and tests
   natively, behind a small `Host` interface.
   - In: `dyamark.setSource(text)`, `setTheme(json)`,
     `scrollToLine(line)`, `setFocus(bool)`. Each call is posted to the
     window's goroutine and wakes it, the same way `App.post` does now.
     Calls made before the first frame are kept and applied in order.
   - Out, through `globalThis.dyamarkHost`: `ready()`, `openLink(url)`,
     `editLine(line)`, `scrolled(line)`, `log(msg)`.
   - Modes: view and viewer, as in the app.
   - Parsing waits 80 ms after the last `setSource`, and diagrams are
     cached by source hash. The IDE sends the whole text on each change.
3. **`bin/wasm`** builds `build/web/`: `dyamark.wasm`, Go's
   `wasm_exec.js` for the same Go version, and `web/index.html` with the
   bridge script.

## The plugin side (Kotlin, `jetbrains/`)

- `plugin.xml`: depends on `org.intellij.plugins.markdown`. Registers
  `<html.panel.provider>` in the `org.intellij.markdown` namespace, plus
  the notification and the settings hook.
- `DyamarkPanelProvider : MarkdownHtmlPanelProvider`. Its name is
  `dyamark`. It's available when `JBCefApp.isSupported()`, and keeps
  the default source text preprocessor, so it is given raw Markdown.
- `DyamarkPanel : MarkdownHtmlPanel`, one per preview:
  - A `JBCefBrowser` that loads `https://dyamark/index.html`. A
    `JBCefCustomSchemeHandlerFactory` serves that from the plugin jar,
    with `application/wasm` for the binary, so
    `instantiateStreaming` works.
  - `setHtml(text, offset, path)` sends `setSource` with the text
    JSON-escaped. Until `ready()`, only the newest text is kept.
  - `scrollToMarkdownSrcOffset(offset)` turns the offset into a line
    with the document, then sends `scrollToLine`. `scrolled(line)` from
    dyamark tells the panel's `ScrollListener`s, so "synchronise
    scrolling" works both ways.
  - `editLine(line)` opens the file at that line (`OpenFileDescriptor`)
    and focuses the editor.
  - `openLink(url)`: `BrowserUtil.browse` for the web, the IDE for
    files, relative to `documentPath`.
  - Disposes its browser and JS queries with the panel.
- `ThemeBridge` reads the global editor scheme and sends `setTheme` to
  every open panel when the scheme or the look and feel changes.
- Build: Gradle with the IntelliJ Platform Gradle Plugin 2.x, against the
  local `/opt/phpstorm` so there's no SDK download. It needs a JDK 25:
  `mise use java@25`, or PhpStorm's bundled JBR, which has `javac`.
- `bin/plugin` runs `bin/wasm`, copies `build/web/` into the plugin's
  resources, and runs `./gradlew buildPlugin`. The result is
  `build/dyamark-jetbrains-<version>.zip`, installed with *Settings ›
  Plugins › ⚙ › Install Plugin from Disk*. Its version comes from
  `git describe`, like the package's.

## Steps

Each step is tested before the next one starts.

| # | Step | Tests |
|---|---|---|
| 1 | **Spike in PhpStorm.** A bare plugin with the provider and a panel that loads the spike's wasm, run with `./gradlew runIde` | WebGL works in PhpStorm's JCEF on `WLToolkit`, both with the GPU and with `ide.browser.jcef.gpu.disable`; typing updates the preview; the keys reach dyamark; memory with 1 and with 5 previews open |
| 2 | Move `palette/` and `viewer/` out of `app/` | `bin/test` passes unchanged; the app looks and works the same |
| 3 | `web/` and `cmd/dyamark-web/`, `bin/wasm` | Native tests of `web/` with a fake `Host`: calls before ready, debounce, `scrollToLine`, `editLine` from the viewer. A headless Chromium page loads the wasm, pushes a document and checks a screenshot |
| 4 | The real provider and panel, the scheme handler, the JS queries | Preview shows, follows typing, survives closing and reopening the tab, and is disposed (no leaked browsers in the IDE's JCEF list) |
| 5 | Colours from the editor scheme, live | Light and dark schemes, switched with a preview open; a diagram with its own `%%{init}%%` theme keeps it |
| 6 | Scroll sync both ways, links, `E` | Each link kind; `E` puts the caret on the fence's line; scroll sync on a long document |
| 7 | The one-time notification and the Preview choice | Choosing dyamark, then JCEF again, swaps every open preview |
| 8 | `bin/plugin`, the release workflow adds the zip, README, PLAN.md, screenshots in both schemes | The released zip installs into a clean PhpStorm |

## Time

These are estimates of my own working time in this session, with tests
and screenshots, not calendar time.

| # | Step | Estimate |
|---|---|---|
| 1 | Spike in PhpStorm, Gradle set up | 45–75 min |
| 2 | `palette/` and `viewer/` out of `app/` | 30–45 min |
| 3 | The wasm side and its tests | 60–90 min |
| 4 | Provider, panel, scheme handler, JS queries | 60–90 min |
| 5 | Colours from the scheme | 30–45 min |
| 6 | Scroll sync, links, `E` | 45–60 min |
| 7 | Notification and Preview choice | 20–30 min |
| 8 | Packaging, release, docs | 30–45 min |
| | **Total** | **about 5½–8 hours** |

Step 1 carries most of the risk: whether WebGL is there in PhpStorm's
JCEF on this machine, and whether the keys arrive. If it fails, stop and
take plan B before doing anything else. Done by hand by someone new to
both Gio and IntelliJ plugins, the whole plan is more like 4–6 days.

## Risks

- **WebGL in the IDE's JCEF.** Gio has no non-WebGL backend. If the IDE
  runs JCEF with the GPU off, Chromium only falls back to software WebGL
  (SwiftShader) when it's allowed to; recent Chromium needs
  `--enable-unsafe-swiftshader`, which the IDE takes through the
  `ide.browser.jcef.extra.args` registry key. When WebGL is missing,
  the panel says so and says how to fix it, rather than staying blank.
- **Keys.** The IDE's keymap sees key presses before the browser does.
  Plain keys should reach dyamark, but `Ctrl+=` / `Ctrl+-` and maybe
  `Tab` may be taken by IDE actions. The ones that are get IDE actions
  of their own that forward to the preview, or are dropped.
- **Experimental API.** The provider and panel interfaces may change in
  2026.3 and later. Pin `until-build` to the tested major, and run the
  Plugin Verifier against each new release before widening it.
- **Memory.** Each preview has its own wasm instance (Go heap about
  10 MB plus the module) in a renderer process the IDE shares. Measure in
  step 1. If it's too much with many tabs, dispose hidden previews'
  browsers and rebuild them on show.
- **Fonts.** In the browser there are no system fonts, only the bundled
  Liberation fonts, so scripts they don't cover show as boxes. Later, the
  plugin can send a fallback font from disk.
- **Two Go versions.** `wasm_exec.js` has to come from the Go that built
  the `.wasm`; `bin/wasm` copies it from `$(go env GOROOT)` every time.
- **Size.** About 5 MB added to the plugin zip. Fine for local installs
  and for the Marketplace.

## Plan B: the app beside the IDE

If step 1 shows WebGL can't be had in the IDE's JCEF, the plugin drives the
native app instead. An "Open in dyamark" action, and optionally every
Markdown file opened, starts `dyamark --follow` beside the IDE. It takes
the document's text and caret line on stdin as the IDE changes them, and
sends `editLine` and link clicks back on stdout. The panel in the IDE
then says "shown in dyamark". On Hyprland, a window rule tiles it next to
PhpStorm. This needs a `--follow` mode in `app/` (no editor, no file
watching, no save), and about half the plugin work above.

## Later

- `R` for other Mermaid types: the plugin runs `mmdc` with
  `GeneralCommandLine` and sends the PNG back, cached as in the app.
- A preview for `.mmd` / `.mermaid` files, opening straight into the
  diagram viewer, through a `FileEditorProvider` of its own.
- A setting to follow the Omarchy theme (the plugin reads and watches
  `colors.toml`) instead of the IDE's scheme.
- Find in the preview.
- Fallback fonts sent from the plugin.
- Publishing to the JetBrains Marketplace.
