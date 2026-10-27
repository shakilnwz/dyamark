package draw_test

import (
	"image"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"dyamark/fonts"
	"dyamark/seq"
	"dyamark/seq/draw"
	layoutPkg "dyamark/seq/layout"
	"dyamark/seq/parse"

	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/text"
	"gioui.org/unit"
	"gioui.org/widget/material"
)

func TestDrawAllDiagrams(t *testing.T) {
	files, err := filepath.Glob("../testdata/*.mmd")
	if err != nil {
		t.Fatal(err)
	}
	if len(files) == 0 {
		t.Fatal("no test files found in ../testdata")
	}

	collection, err := fonts.Collection()
	if err != nil {
		t.Fatalf("loading fonts: %v", err)
	}
	th := material.NewTheme()
	th.Shaper = text.NewShaper(text.WithCollection(collection), text.NoSystemFonts())
	seqTh := seq.DefaultTheme()

	for _, path := range files {
		name := filepath.Base(path)
		t.Run(name, func(t *testing.T) {
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("reading %s: %v", path, err)
			}

			ast, err := parse.Parse(strings.TrimSpace(string(data)))
			if err != nil {
				t.Fatalf("parsing %s: %v", path, err)
			}

			diag, err := layoutPkg.Layout(ast, layoutPkg.Default())
			if err != nil {
				t.Fatalf("layout %s: %v", path, err)
			}

			ops := new(op.Ops)
			gtx := layout.Context{
				Ops: ops,
				Metric: unit.Metric{
					PxPerDp: 1.0,
					PxPerSp: 1.0,
				},
				Constraints: layout.Exact(image.Pt(int(diag.ViewBox.W), int(diag.ViewBox.H))),
			}

			dims := draw.Draw(gtx, th, seqTh, diag)
			if dims.Size.X <= 0 || dims.Size.Y <= 0 {
				t.Errorf("got invalid dimensions %v for %s", dims.Size, name)
			}
		})
	}
}

func TestDrawNil(t *testing.T) {
	dims := draw.Draw(layout.Context{}, nil, seq.DefaultTheme(), nil)
	if dims.Size != (image.Point{}) {
		t.Errorf("expected zero dimensions for nil diagram, got %v", dims.Size)
	}
}
