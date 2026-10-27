package app

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"dyamark/markdown"
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
