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
		lastGood := c.lastGood[blockIndex]
		if cached.Error == nil {
			c.lastGood[blockIndex] = cached.Diagram
			lastGood = cached.Diagram
		}
		return cached.AST, cached.Diagram, cached.Error, lastGood
	}

	ast, err := parse.Parse(source)
	if err != nil {
		c.diagrams[hash] = &CachedDiagram{Error: err}
		return nil, nil, err, c.lastGood[blockIndex]
	}

	diag, err := layoutPkg.Layout(ast, layoutPkg.Default())
	if err != nil {
		c.diagrams[hash] = &CachedDiagram{AST: ast, Error: err}
		return ast, nil, err, c.lastGood[blockIndex]
	}

	c.diagrams[hash] = &CachedDiagram{AST: ast, Diagram: diag}
	c.lastGood[blockIndex] = diag
	return ast, diag, nil, diag
}

// RenderMmdc renders a non-sequence Mermaid diagram with mmdc, caching the
// output PNG on disk in ~/.cache/dyamark/.
func (c *DiagramCache) RenderMmdc(source string) (image.Image, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	hash := fmt.Sprintf("%x", sha256.Sum256([]byte(source)))
	if img, ok := c.pngCache[hash]; ok {
		return img, nil
	}

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
				c.pngCache[hash] = img
				return img, nil
			}
		}
	}

	mmdcPath, err := exec.LookPath("mmdc")
	if err != nil {
		return nil, fmt.Errorf("mmdc not found in PATH")
	}

	tmpInput := filepath.Join(dyamarkCache, "tmp-"+hash+".mmd")
	if err := os.WriteFile(tmpInput, []byte(source), 0o644); err != nil {
		return nil, err
	}
	defer os.Remove(tmpInput)

	cmd := exec.Command(mmdcPath, "-i", tmpInput, "-o", pngPath, "-b", "transparent")
	if out, err := cmd.CombinedOutput(); err != nil {
		return nil, fmt.Errorf("mmdc render error: %s: %w", string(out), err)
	}

	f, err := os.Open(pngPath)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	img, err := png.Decode(f)
	if err != nil {
		return nil, err
	}

	c.pngCache[hash] = img
	return img, nil
}
