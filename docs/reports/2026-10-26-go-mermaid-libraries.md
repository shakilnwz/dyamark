# Report: existing Go modules for Mermaid sequence diagrams

Date: 2026-10-26

**Question:** is there a Go module dyamark can use instead of writing its own
Mermaid sequence-diagram parser, layout and renderer?

**Short answer:** no module gives Mermaid-matching layout or draws to Gio.
One parser, `julianshen/mmgo`'s `pkg/parser/sequence`, is a good base to copy
into the repo and extend. Layout and drawing stay ours.

## Method

- Searched GitHub (`gh search repos`, Go language) and the web for pure-Go
  Mermaid renderers.
- Downloaded the four licensed candidates into the Go module cache and read
  their sequence-diagram source: data model, parser, refusals, layout
  constants, tests, imports.
- **Not done:** running them. Executing downloaded third-party code was
  blocked by the session's permission check, so there is no side-by-side
  visual comparison yet. A reference render of a sample diagram made with
  `mmdc` exists, ready for that comparison if running the libraries is
  allowed later.

## Candidates

| Module | Version / last push | Licence | Sequence code | Output |
|---|---|---|---|---|
| [julianshen/mmgo](https://github.com/julianshen/mmgo) | 2026-05-16 commit | MIT | Parser 621 lines + data model 250 lines (1,362 lines of parser tests, 80 cases); renderer separate | SVG, PNG, PDF |
| [nlink-jp/mermaid-render](https://github.com/nlink-jp/mermaid-render) | v0.6.0, 2026-09-30 | MIT | Lexer + parser ~940 lines, model 111 lines; raster renderer 767 lines | PNG, text |
| [zkrebbekx/go-mermaid](https://github.com/zkrebbekx/go-mermaid) | v0.1.6, 2026-10-17 | MIT | ~880 lines | SVG (+ raster subpackage) |
| [luo-studio/go-mermaid](https://github.com/luo-studio/go-mermaid) | one commit, 2026-05-08 | MIT | ~820 lines | Display list → PNG / SVG / PDF |
| [bvolpato/mermaid-go-renderer](https://github.com/bvolpato/mermaid-go-renderer) | 2026-10-21 | **None** | — | SVG / PNG |

All five were created in 2026, have 0–10 stars and a single maintainer.

## Findings

### No module reproduces Mermaid's layout

None ports Mermaid's `sequenceRenderer.ts` constants (actor 150×65,
`actorMargin` 50, `messageMargin` 35, `boxMargin` 10, `noteMargin` 10). mmgo,
for example, uses its own (`defaultRowHeight` 50, `defaultBoxHeight` 35). Using
any of them as the renderer would give a reasonable diagram, but not one that
lines up with Mermaid's.

### No module draws to Gio

They produce SVG, PNG or PDF. Showing one of their images inside Gio would
work for a first version, but zooming would blur, and the pinned participant
row and focus-a-participant would be impossible, since there's no geometry
to work with. luo-studio's display list is the closest to drawable, but it's
one unmaintained commit.

### Parsers: mmgo keeps what a faithful renderer needs, nlink-jp drops it

**nlink-jp/mermaid-render** is built to "draw what it reads and refuse what
it cannot read". Its parser comments say so:

- **Refused**, so the diagram falls back to source: `create` / `destroy`,
  participant configs `@{ "type": … }`, half arrows, central connections,
  HTML in labels.
- **Dropped:** box colours, `rect` frames and colours (only the contents are
  kept), `wrap:` prefixes, `link` / `links` / `properties` / `details`,
  `accTitle`.
- Blocks are a flat event list (start / section / end), and the model has
  no `rect` kind.

That's right for its purpose (terminal transcripts), but wrong for dyamark.

**julianshen/mmgo**'s parser keeps the presentation data:

- Participants with kind (participant / actor), alias, box index, and the
  item at which each is created or destroyed.
- Boxes with fill colour and alpha.
- Every arrow type, including bidirectional `<<->>` / `<<-->>`, and `+` /
  `-` activation suffixes.
- Blocks as a tree (`alt`, `opt`, `loop`, `par`, `critical`, `break`,
  **`rect`**) with branches and fill colour.
- Notes left of / right of / over one or two participants.
- `autonumber` with start and step, title, `accTitle` / `accDescr`,
  frontmatter title.
- Errors with 1-based line numbers.
- **Standard library only.** It imports `diagram` and a small `parser`
  utility package from the same repo, and nothing else.

Gaps in mmgo's parser:

- No `%%{init}%%` / frontmatter config (theme, `mirrorActors`, margins) and
  no `wrap`. dyamark can read those itself before handing over the diagram.
- No v11 typed participant shapes (only participant / actor).
- No per-item source line numbers in the model, only in errors.
- Five months without a commit.

### Other notes

- bvolpato/mermaid-go-renderer has no licence file, so it can't legally be
  used or copied.
- riclib/mmdr-go (seen in search results) links a Rust engine through cgo,
  so it isn't pure Go.

## Recommendation

1. **Copy mmgo's sequence parser into dyamark** (`seq/parse`, about 870
   lines with its data model, plus its 80 tests), keeping its MIT licence as
   `seq/parse/LICENSE.mmgo` and noting the upstream commit. Copy rather than
   depend: upstream is quiet, and we need to extend it with config, `wrap`,
   `<br>`, per-item line numbers and later the typed shapes.
2. **Write layout and drawing ourselves** as planned: a port of Mermaid's
   `sequenceRenderer.ts`, drawn with Gio, and checked against `mmdc`
   references.
3. **Use nlink-jp's and mmgo's test inputs as extra parity cases**, alongside
   Mermaid's own spec cases.
4. If running third-party code is allowed later, render the sample diagram
   with each module and with `mmdc` to confirm the layout findings visually.

## Earlier measurements in this session (for context)

- Go + WebKit (webview_go, goldmark, fsnotify), stripped: 3.0 MB without
  Mermaid, 4.5 MB with Mermaid embedded gzipped, 8.3 MB with it embedded
  as-is. `mermaid.min.js` (11.x) is 5.3 MB, 1.5 MB gzipped. Not chosen,
  because of WebKit's memory and start-up cost.
- omawrite: C++17 + Qt Quick, ~4,900 lines, ~1 MB installed, a styled-source
  editor with no rendered preview.
