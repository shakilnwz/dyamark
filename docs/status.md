# Status

Where dyamark stands against [the plan](plan.md), phase by phase.
Last updated 2026-11-07, at v1.5.0.

| Phase | | State |
|---|---|---|
| 0 | Spike | Done |
| 1 | Parser | Done |
| 2 | Layout, SVG, parity | Done |
| 3 | Drawing, viewer | Done |
| 4 | Markdown view | Done |
| 5 | Editing | Done |
| 6 | Sequence extras | Done |
| 7 | Packaging | Mostly done: no screenshot of the app in the README yet |
| 8 | PhpStorm / JetBrains plugin | Shipped in v1.3.0 |
| 9 | Later | Not started |

## Releases

| Tag | Date | Assets |
|---|---|---|
| v1.4.0 | 2026-11-03 | Arch `.pkg.tar.zst`, `.deb`, `.rpm`, Windows `.exe`, JetBrains plugin `.zip` |
| v1.3.0 | 2026-11-02 | Arch `.pkg.tar.zst`, `.deb`, `.rpm`, Windows `.exe`, JetBrains plugin `.zip` |
| v1.2.0 | 2026-10-28 | Arch, `.deb`, `.rpm`, Windows |
| v1.1.0 | 2026-10-28 | Arch, `.deb`, `.rpm`, Windows |
| v1.0.0 | 2026-10-28 | Arch, `.deb`, `.rpm`, Windows |

All five were built and published by `.github/workflows/release.yml`.

## Phase notes

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

**Phase 4 (Markdown view):** done. The whole document is drawn, with bold,
italics, code spans and links (a clicked link opens in the desktop's app
for it, or here if it's a Markdown file), sequence diagrams inline, parse
errors over the last good drawing, other Mermaid types as source with `R`
to render them through `mmdc`, and live reload when the file changes.

**Phase 5 (editing):** done.

- `Ctrl+E` shows the source, `Ctrl+\` puts it beside the rendered view,
  which follows the typing 80 ms after it stops and scrolls to the block
  the caret is in. Only the diagrams that changed are laid out again.
- A faint toolbar along the bottom has buttons for save, open, source,
  source and view, find and the key list, each with a tooltip naming its
  key. After them it says what last happened to the file ("Saved plan.md",
  "Unsaved changes to plan.md"; a failed save stays there in red until one
  works, and errors show there for a few seconds). On the right are the
  caret's line and column while editing, and the word count.
- Saves are atomic, and a document with a file saves itself a second after
  typing stops. Untitled drafts are only saved when asked.
- A change on disk is taken in quietly when there are no edits here, and
  otherwise asks which version to keep.
- `Ctrl+O` and `Ctrl+Shift+S` use the desktop's file dialogs
  (xdg-desktop-portal). Closing the window with an unsaved draft asks first.
- `E` in the viewer jumps to that diagram's source.

**Phase 6 (sequence extras):** done. In the viewer, clicking a
participant dims every message, note and activation it isn't part of;
clicking it again or `Esc` shows everything. Scrolled down a tall
diagram, the participants' names stay pinned at the top in a thin strip,
each over its lifeline, as small at any zoom; a name too long for the room
is cut short and shows in full under the pointer. Clicking a pinned name
focuses that participant too.

Memory is over the plan's budget: about 105–112 MB RSS, mostly Mesa's
libraries; Go's own heap is about 10 MB.

**Phase 7 (packaging):** mostly done.

- `pkgbuild/PKGBUILD`, `dyamark.desktop` and the `dyamark.svg` icon;
  `bin/install` builds and installs the Arch package locally.
- The `?` / `F1` key list, with the fixes from
  [the UI review](reports/2026-10-28-ui-review.md). Five low findings
  from that review are still open.
- Pushing a `v*` tag runs the release workflow: tests, then an Arch
  package, a `.deb` and an `.rpm` (made by nfpm from `pkgbuild/nfpm.yaml`),
  and a Windows `.exe`. The Linux packages need glibc 2.39 or newer.
- The Windows build has no file dialog (the dialogs go through
  xdg-desktop-portal), so files are opened from the command line or
  "Open with", and an untitled draft can't be saved.
- Not done: a screenshot of the app itself, from sample content, in the
  README. The README's screenshots are of the PhpStorm plugin.

**Phase 8 (PhpStorm / JetBrains plugin):** shipped in v1.3.0. Steps 1–8 of
[its plan](plans/2026-11-01-phpstorm-plugin.md) are done and steps 4–7
were checked in PhpStorm with `./gradlew runIde -Pcheck`. The release
workflow built the plugin zip for v1.3.0. v1.4.0 makes the preview
smooth: 60 fps instead of JCEF's off-screen 30, eased wheel scrolling,
cached diagrams, eased zoom in the viewer, text zoom that keeps the
place, and scroll sync in fractions of a line. Not checked yet: the zip
installed into a clean PhpStorm, and a touchpad in the IDE.

v1.5.0: in the viewer, scrolling moves the diagram
and Ctrl+scrolling zooms it, in the app and the plugin. Scrolling a document
is smoother: notches ease on a critically damped spring, now in the app too,
and in the plugin the editor only follows the newest place. The plugin has no
upper version limit and works on 2026.3 (build 263). All 37 checks of
`./gradlew runIde -Pcheck` pass on 2026.2.4 and 2026.3, and the Plugin
Verifier finds the zip built against 2026.2.4 compatible with both. The
verifier now runs in the release workflow and every week.

## Planned next

- [Editing a block in place, with Vim-style modes](plans/2026-10-28-inline-editing.md):
  planned, not started.
- Phase 9 of the plan: the v11 typed participant shapes (they parse, and
  draw as plain boxes for now), editor syntax colouring, and a headless
  `check` / `export` CLI on top of `seq/svg`.
