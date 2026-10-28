package app

import (
	"image"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"dyamark/fonts"
	"dyamark/markdown"

	"gioui.org/f32"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/text"
	"gioui.org/unit"
	"gioui.org/widget/material"
)

const markdownWithFences = `# Title

Some text here.

` + "```mermaid" + `
sequenceDiagram
Alice->>Bob: Hello
` + "```" + `

Middle text.

` + "```mermaid" + `
sequenceDiagram
Bob->>Alice: World
` + "```" + `
`

func TestExtractDiagrams(t *testing.T) {
	cache := markdown.NewDiagramCache()
	doc := markdown.Parse([]byte(markdownWithFences), cache)
	if len(doc.Sequences) != 2 {
		t.Fatalf("expected 2 diagrams from markdown, got %d", len(doc.Sequences))
	}

	docEmpty := markdown.Parse([]byte("just regular markdown without mermaid blocks"), cache)
	if len(docEmpty.Sequences) != 0 {
		t.Errorf("expected 0 diagrams, got %d", len(docEmpty.Sequences))
	}
}

func TestDefaultSampleMarkdownParses(t *testing.T) {
	cache := markdown.NewDiagramCache()
	doc := markdown.Parse([]byte(DefaultSampleMarkdown), cache)
	if len(doc.Sequences) != 1 {
		t.Fatalf("expected 1 diagram from DefaultSampleMarkdown, got %d", len(doc.Sequences))
	}
	if doc.Sequences[0].AST == nil || doc.Sequences[0].AST.Title != "Authentication Sequence Flow" {
		t.Errorf("expected title 'Authentication Sequence Flow', got %v", doc.Sequences[0].AST)
	}
}

func TestFileWatcher(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "test.md")
	if err := os.WriteFile(filePath, []byte("# Initial"), 0o644); err != nil {
		t.Fatal(err)
	}

	var mu sync.Mutex
	reloaded := make(chan string, 4)
	fw, err := WatchFile(filePath, func(data []byte) {
		mu.Lock()
		reloaded <- string(data)
		mu.Unlock()
	})
	if err != nil {
		t.Fatal(err)
	}
	defer fw.Close()

	// Modify file
	time.Sleep(20 * time.Millisecond)
	if err := os.WriteFile(filePath, []byte("# Updated"), 0o644); err != nil {
		t.Fatal(err)
	}

	select {
	case content := <-reloaded:
		if content != "# Updated" {
			t.Errorf("expected '# Updated', got %q", content)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for file reload")
	}
}

func TestRSSKB(t *testing.T) {
	rss, err := RSSKB()
	if err != nil {
		t.Fatalf("RSSKB failed: %v", err)
	}
	if rss == 0 {
		t.Error("expected non-zero RSS")
	}
}

func TestAppOpenFile(t *testing.T) {
	dir := t.TempDir()
	mdPath := filepath.Join(dir, "notes.md")
	if err := os.WriteFile(mdPath, []byte("# Note Title\n\nSome body text"), 0o644); err != nil {
		t.Fatal(err)
	}

	appInstance, err := New(Options{FilePath: mdPath})
	if err != nil {
		t.Fatalf("New app failed: %v", err)
	}
	if appInstance.mode != ModeInline {
		t.Errorf("expected ModeInline for regular markdown, got %v", appInstance.mode)
	}
	if len(appInstance.doc.Blocks) == 0 {
		t.Fatalf("expected parsed blocks, got 0")
	}

	// Open a pure sequence diagram
	mmdPath := filepath.Join(dir, "diagram.mmd")
	if err := os.WriteFile(mmdPath, []byte("sequenceDiagram\nAlice->>Bob: Hello"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := appInstance.OpenFile(mmdPath); err != nil {
		t.Fatalf("OpenFile failed: %v", err)
	}
	if appInstance.mode != ModeViewer {
		t.Errorf("expected ModeViewer for pure .mmd, got %v", appInstance.mode)
	}
	if appInstance.viewer.Diagram == nil {
		t.Errorf("expected viewer to have laid-out diagram")
	}
}

// Reloads arrive on the watcher's goroutine; they must wait for the UI
// goroutine instead of swapping the document under a frame.
func TestReloadRunsOnUIGoroutine(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "a.md")
	if err := os.WriteFile(p, []byte("# one\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	a, err := New(Options{FilePath: p})
	if err != nil {
		t.Fatal(err)
	}
	defer a.fileWatcher.Close()
	before := a.doc

	if err := os.WriteFile(p, []byte("# two\n\ntext\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(2 * time.Second)
	for {
		a.mu.Lock()
		n := len(a.pending)
		a.mu.Unlock()
		if n > 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("no reload queued")
		}
		if a.doc != before {
			t.Fatal("document replaced off the UI goroutine")
		}
		time.Sleep(5 * time.Millisecond)
	}

	a.runPending()
	if a.doc == before || len(a.doc.Blocks) != 2 {
		t.Fatalf("reload not applied: %d blocks", len(a.doc.Blocks))
	}
}

// Saving the file keeps the viewer's zoom, pan and sequence-number toggle;
// opening another file fits its diagram afresh.
func TestReloadKeepsViewerPlace(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "a.mmd")
	if err := os.WriteFile(p, []byte(sampleMMD1), 0o644); err != nil {
		t.Fatal(err)
	}
	a, err := New(Options{FilePath: p})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { a.fileWatcher.Close() }()

	collection, err := fonts.Collection()
	if err != nil {
		t.Fatal(err)
	}
	mTheme := material.NewTheme()
	mTheme.Shaper = text.NewShaper(text.WithCollection(collection), text.NoSystemFonts())
	gtx := layout.Context{
		Ops:         new(op.Ops),
		Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
		Constraints: layout.Exact(image.Pt(800, 600)),
	}
	frame := func() {
		gtx.Reset()
		a.viewer.Layout(gtx, mTheme, DefaultTheme(10))
	}
	frame()

	a.viewer.ZoomBy(2, f32.Pt(100, 100))
	a.viewer.ToggleSequenceNumbers()
	zoom, pan := a.viewer.Zoom, a.viewer.Pan
	if !a.viewer.Config.ShowSequenceNumbers {
		t.Fatal("N didn't turn sequence numbers on")
	}

	a.reload([]byte(sampleMMD1 + "\nBob->>Alice: again\n"))
	frame()
	if a.viewer.Zoom != zoom || a.viewer.Pan != pan {
		t.Errorf("reload moved the view: zoom %f pan %v, want %f %v", a.viewer.Zoom, a.viewer.Pan, zoom, pan)
	}
	if !a.viewer.Config.ShowSequenceNumbers {
		t.Error("reload undid N")
	}
	if len(a.viewer.Diagram.Messages) != 3 {
		t.Errorf("reload not drawn: %d messages, want 3", len(a.viewer.Diagram.Messages))
	}

	q := filepath.Join(dir, "b.mmd")
	if err := os.WriteFile(q, []byte(sampleMMD2), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := a.OpenFile(q); err != nil {
		t.Fatal(err)
	}
	frame()
	if a.viewer.Zoom == zoom {
		t.Error("another file kept the old zoom instead of fitting")
	}
}

// A broken diagram in a newly opened file has no last good drawing to show
// under its error; the previous file's diagram isn't one.
func TestOpenFileForgetsLastGood(t *testing.T) {
	dir := t.TempDir()
	first := filepath.Join(dir, "first.md")
	second := filepath.Join(dir, "second.md")
	if err := os.WriteFile(first, []byte("```mermaid\n"+sampleMMD1+"\n```\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(second, []byte("```mermaid\nsequenceDiagram\nloop Unclosed\nA->>B: hi\n```\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	a, err := New(Options{FilePath: first})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { a.fileWatcher.Close() }()
	if err := a.OpenFile(second); err != nil {
		t.Fatal(err)
	}
	s := a.doc.Sequences[0]
	if s.Error == nil {
		t.Fatal("expected the diagram to fail")
	}
	if s.LastGood != nil {
		t.Error("the previous file's diagram is shown as this one's last good drawing")
	}
}

// A reload queued for a file that has since been replaced is dropped.
func TestStaleReloadIgnored(t *testing.T) {
	dir := t.TempDir()
	first := filepath.Join(dir, "first.md")
	second := filepath.Join(dir, "second.md")
	if err := os.WriteFile(first, []byte("# first\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(second, []byte("# second\n\nbody\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	a, err := New(Options{FilePath: first})
	if err != nil {
		t.Fatal(err)
	}
	stale := a.fileWatcher
	if err := a.OpenFile(second); err != nil {
		t.Fatal(err)
	}
	defer a.fileWatcher.Close()

	stale.onReload([]byte("# first again\n"))
	a.runPending()
	if len(a.doc.Blocks) != 2 {
		t.Fatalf("stale reload replaced the new file: %d blocks", len(a.doc.Blocks))
	}
}

// Clicking a diagram opens that diagram, even when an earlier one failed
// to parse and so isn't in the viewer's list.
func TestOpenViewerSkipsBrokenDiagrams(t *testing.T) {
	src := "```mermaid\nsequenceDiagram\nA-\n```\n\n" +
		"```mermaid\nsequenceDiagram\nA->>B: first\n```\n\n" +
		"```mermaid\nsequenceDiagram\nC->>D: second\n```\n"
	p := filepath.Join(t.TempDir(), "b.md")
	if err := os.WriteFile(p, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	a, err := New(Options{FilePath: p})
	if err != nil {
		t.Fatal(err)
	}
	defer a.fileWatcher.Close()
	if a.doc.Sequences[0].AST != nil {
		t.Fatal("expected the first diagram to fail to parse")
	}

	a.mdView.OnOpenViewer(a.doc.Sequences[1])
	if a.viewer.AST != a.doc.Sequences[1].AST {
		t.Errorf("opened diagram %d, want the clicked one", a.viewer.CurIndex)
	}
}
