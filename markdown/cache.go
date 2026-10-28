package markdown

import (
	"crypto/sha256"
	"fmt"
	"image"
	"image/png"
	"os"
	"os/exec"
	"path/filepath"
	"sync"

	layoutPkg "dyamark/seq/layout"
	"dyamark/seq/parse"
)

// DiagramCache caches parsed and laid-out diagrams by source hash, and
// tracks the last-known good layout for each diagram position.
type DiagramCache struct {
	mu       sync.Mutex
	diagrams map[string]*CachedDiagram
	lastGood map[int]*layoutPkg.Diagram
	pngCache map[string]image.Image
}

// CachedDiagram holds the result of parsing and laying out a sequence diagram.
type CachedDiagram struct {
	AST     *parse.Diagram
	Diagram *layoutPkg.Diagram
	Error   error

	// used is set when the diagram is asked for, and cleared by Prune.
	used bool
}

// NewDiagramCache creates an empty diagram cache.
func NewDiagramCache() *DiagramCache {
	return &DiagramCache{
		diagrams: make(map[string]*CachedDiagram),
		lastGood: make(map[int]*layoutPkg.Diagram),
		pngCache: make(map[string]image.Image),
	}
}

// GetOrLayout retrieves a cached diagram or parses and lays it out.
func (c *DiagramCache) GetOrLayout(source string, blockIndex int) (*parse.Diagram, *layoutPkg.Diagram, error, *layoutPkg.Diagram) {
	c.mu.Lock()
	defer c.mu.Unlock()

	hash := fmt.Sprintf("%x", sha256.Sum256([]byte(source)))
	if cached, ok := c.diagrams[hash]; ok {
		cached.used = true
		lastGood := c.lastGood[blockIndex]
		if cached.Error == nil {
			c.lastGood[blockIndex] = cached.Diagram
			lastGood = cached.Diagram
		}
		return cached.AST, cached.Diagram, cached.Error, lastGood
	}

	ast, err := parse.Parse(source)
	if err != nil {
		c.diagrams[hash] = &CachedDiagram{Error: err, used: true}
		return nil, nil, err, c.lastGood[blockIndex]
	}

	diag, err := layoutPkg.Layout(ast, layoutPkg.Default())
	if err != nil {
		c.diagrams[hash] = &CachedDiagram{AST: ast, Error: err, used: true}
		return ast, nil, err, c.lastGood[blockIndex]
	}

	c.diagrams[hash] = &CachedDiagram{AST: ast, Diagram: diag, used: true}
	c.lastGood[blockIndex] = diag
	return ast, diag, nil, diag
}

// Prune forgets the diagrams not asked for since the last Prune. Called
// after each parse of the document being edited, it keeps only the
// current diagrams, not every version typed on the way.
func (c *DiagramCache) Prune() {
	c.mu.Lock()
	defer c.mu.Unlock()
	for hash, d := range c.diagrams {
		if !d.used {
			delete(c.diagrams, hash)
			continue
		}
		d.used = false
	}
}

// ClearLastGood forgets the last good layout of every diagram position. Call
// it when another file is opened, whose diagrams those layouts aren't.
func (c *DiagramCache) ClearLastGood() {
	c.mu.Lock()
	defer c.mu.Unlock()
	clear(c.lastGood)
}

// Rendered returns the image mmdc made of source earlier in this run, or
// nil. It only looks in memory, so it's cheap enough for every parse.
func (c *DiagramCache) Rendered(source string) image.Image {
	hash := fmt.Sprintf("%x", sha256.Sum256([]byte(source)))
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.pngCache[hash]
}

// RenderMmdc renders a non-sequence Mermaid diagram with mmdc, caching the
// output PNG on disk in ~/.cache/dyamark/.
// The cache lock isn't held while mmdc runs, which can take seconds, so
// parsing and layout aren't blocked meanwhile.
func (c *DiagramCache) RenderMmdc(source string) (image.Image, error) {
	hash := fmt.Sprintf("%x", sha256.Sum256([]byte(source)))
	c.mu.Lock()
	img, ok := c.pngCache[hash]
	c.mu.Unlock()
	if ok {
		return img, nil
	}

	img, err := renderMmdc(source, hash)
	if err != nil {
		return nil, err
	}
	c.mu.Lock()
	c.pngCache[hash] = img
	c.mu.Unlock()
	return img, nil
}

// renderMmdc loads the PNG for hash from the disk cache, or makes it with
// mmdc.
func renderMmdc(source, hash string) (image.Image, error) {
	cacheDir, err := os.UserCacheDir()
	if err != nil {
		cacheDir = os.TempDir()
	}
	dyamarkCache := filepath.Join(cacheDir, "dyamark")
	if err := os.MkdirAll(dyamarkCache, 0o755); err != nil {
		return nil, err
	}

	pngPath := filepath.Join(dyamarkCache, hash+".png")
	if _, err := os.Stat(pngPath); err == nil {
		f, err := os.Open(pngPath)
		if err == nil {
			defer f.Close()
			img, err := png.Decode(f)
			if err == nil {
				return img, nil
			}
		}
	}

	mmdcPath, err := exec.LookPath("mmdc")
	if err != nil {
		return nil, fmt.Errorf("mmdc not found in PATH")
	}

	tmp, err := os.CreateTemp(dyamarkCache, "tmp-*.mmd")
	if err != nil {
		return nil, err
	}
	tmpInput := tmp.Name()
	defer os.Remove(tmpInput)
	_, err = tmp.WriteString(source)
	if cerr := tmp.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return nil, err
	}

	cmd := exec.Command(mmdcPath, "-i", tmpInput, "-o", pngPath, "-b", "transparent")
	if out, err := cmd.CombinedOutput(); err != nil {
		return nil, fmt.Errorf("mmdc render error: %s: %w", string(out), err)
	}

	f, err := os.Open(pngPath)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	return png.Decode(f)
}
