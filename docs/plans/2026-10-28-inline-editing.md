# Plan: editing a block in place, with Vim-style modes

Date: 2026-10-28. Status: planned, not started.

## Goal

Edit one block of the document where it stands, while every other block
stays rendered. Two modes, as in Vim, but only the simple part of Vim:

- **Normal** is the rendered view, as it is today. Reading, scrolling,
  links, diagrams and every existing key work as now.
- **Insert** shows one block's Markdown source in place, with a caret,
  and the rest of the document rendered around it.

| Key (in Normal) | Does |
|---|---|
| `i` | Edit the current block, caret at its start |
| `a` | Edit the current block, caret at its end |
| `o` | Open a new, empty block below the current one and edit it |
| `Esc` (in Insert) | Back to Normal: the block renders again |

Nothing else changes: the arrows, `PgUp`/`PgDn`, `Space`, `Home`/`End`,
`Tab`, `Enter`, `R`, `?`, the `Ctrl` keys and clicks on links and diagrams
all do what they do today. A click never starts editing by itself, so a
file opened to read can't be changed by a stray click and some typing.

## Decisions

- Simple modes only: `i`, `a`, `o` and `Esc`. No `hjkl`, counts,
  operators or `:` commands.
- In Insert the arrow keys stay inside the block. To edit another block,
  press `Esc`, choose it, and press `i`, `a` or `o` again.
- A diagram's source is edited in place like any other block. There's no
  live drawing under it: the diagram is drawn again after `Esc`. To see
  a diagram change as you type, use the source-and-view split
  (`Ctrl+\`), as now.
- The caret goes to the block's start (`i`) or end (`a`). Putting it where
  you clicked is left for later.
- Insert works in the rendered view only. Split view already has the
  source on the left, and the diagram viewer keeps its own keys (`E`
  still edits a diagram's source).

## The current block

`i`, `a` and `o` act on the *current block* in Normal:

1. the block last clicked, if it's still on screen;
2. otherwise the first block whose top is on screen.

The current block shows a faint bar at its left, in the foreground colour
at low strength, so it doesn't look like the accent-coloured find match.
After `Esc`, the block just edited stays current. In an empty document,
`i`, `a` and `o` start a block at the end.

Clicking a diagram still opens it in the viewer, and also makes it the
current block.

## How it works

1. **Source ranges.** The parser records where each block's source starts
   and ends in the file, as byte offsets: a fenced block from its opening
   fence to its closing one, a list or table as a whole. Today it keeps
   only each block's first line, which isn't enough.
2. **One small editor for the block.** A second `widget.Editor` holds the
   block's source and is laid out in the list in place of that block. It
   uses the monospace source font on a faint band, with an accent bar at
   its left, so it's clear which part is source.
3. **The whole text stays the one source of truth.** Every change in the
   block's editor is written into the main editor's text, replacing the
   block's range in one step (`SetCaret` over the range, then `Insert`).
   So saving, auto-save, "Unsaved changes", find, the word count and the
   toolbar's line and column go on working unchanged. The range's end
   moves with the typing.
4. **Half-typed Markdown can't break the page.** While a block is in
   Insert, the text before it and the text after it are parsed apart.
   An opening ` ``` ` typed without its closing one would otherwise turn
   everything below into code until it's closed. After `Esc`, the whole
   document is parsed again as usual.
5. **The block can grow.** What's being edited is a range of text, not a
   fixed block: a blank line and a second paragraph, or three pasted
   sections, all render after `Esc`.
6. **`o`** inserts a blank line after the current block's range and
   starts Insert on the empty line after it, so the new text becomes its
   own block.

## Living with what's there

- **Toolbar.** The right side says `Insert` in the accent colour while a
  block is being edited, before the line and column, which count from the
  top of the file. Normal shows the word count only, as now.
- **Find (`Ctrl+F`).** A match inside the block in Insert is selected
  there; showing a match elsewhere first goes back to Normal.
- **`Ctrl+E` and `Ctrl+\`.** From Insert they open the full source, with
  the caret where it was in the block.
- **Changed on disk.** With no unsaved edits, the block goes back to
  Normal and the file is reloaded. With edits, the prompt asks which
  version to keep, as now.
- **Undo.** `Ctrl+Z` undoes typing inside the block. Undoing a block after
  leaving it needs one shared history and is left for later.
- **Key list (`?`).** A "Normal and Insert" group lists `i`, `a`, `o` and
  `Esc`.

## Steps

Each step is tested before the next one starts.

| # | Step | Tests |
|---|---|---|
| 1 | Source ranges in the parser | Every block type, `.mmd` files, rules and blocks with no text of their own |
| 2 | Current block: the click and top-of-screen rules, the bar | Which block is current after clicks, scrolling and `Esc` |
| 3 | Insert with `i` and `a`, `Esc` back, the change written into the whole text | The file after editing, auto-save, the toolbar's text, caret at start and end |
| 4 | `o`, and an empty document | The new block's place in the file |
| 5 | Parsing before and after the block apart | An unclosed fence leaves the blocks below rendered |
| 6 | Diagram blocks in Insert, drawn again after `Esc` | A diagram edited and drawn again; a broken one shows its error |
| 7 | Find, `Ctrl+E`/`Ctrl+\`, changed on disk, toolbar, key list | One test for each |
| 8 | Screenshots in both themes; README, PLAN.md, key list | |

## Time

These are estimates of my own working time in this session, with tests
and screenshots, not calendar time.

| # | Step | Estimate |
|---|---|---|
| 1 | Source ranges | 20–30 min |
| 2 | Current block | 20–30 min |
| 3 | Insert with `i`, `a`, `Esc`, writing back | 45–60 min |
| 4 | `o` and the empty document | 15–20 min |
| 5 | Parsing apart while editing | 25–35 min |
| 6 | Diagram blocks | 10–15 min |
| 7 | Find, `Ctrl+E`/`Ctrl+\`, disk changes, toolbar, keys | 30–40 min |
| 8 | Screenshots and docs | 20–30 min |
| | **Total** | **about 3–4½ hours** |

Step 3 carries most of the risk: it's where a second editor inside the
scrolling list has to keep the keyboard, scroll with the page and stay in
step with the whole text through reparses. If it goes badly, it's the
step that grows. Done by hand by someone new to the code, the whole plan
is more like 2–3 days.

## Later

- The caret where you click.
- `O` (open above), `I`, `A`, and moving between blocks with the arrows
  in Insert.
- One undo history across blocks.
- Source in the block styled as it renders: headings at heading size,
  bold in bold.
