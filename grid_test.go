package ultrahdr

import (
	"bytes"
	"image"
	"image/color"
	"io"
	"os"
	"path/filepath"
	"testing"
)

func TestGridSDR(t *testing.T) {
	paths := []string{
		"testdata/sample_srgb.jpg",
		"testdata/sample_display_p3.jpg",
		"testdata/sample_adobe_rgb.jpg",
	}

	readers := make([]io.Reader, 0, len(paths))
	files := make([]*os.File, 0, len(paths))
	for _, p := range paths {
		f, err := os.Open(p)
		if err != nil {
			t.Fatalf("open %s: %v", p, err)
		}
		files = append(files, f)
		readers = append(readers, f)
	}
	for _, f := range files {
		defer f.Close()
	}

	res, err := Grid(readers, 2, 400, 300, &GridOptions{
		Quality:       85,
		Interpolation: InterpolationLanczos2,
		Background:    color.White,
	})
	if err != nil {
		t.Fatalf("grid: %v", err)
	}
	if res == nil || res.Primary == nil {
		t.Fatalf("missing result")
	}
	cfg, _, err := image.DecodeConfig(bytes.NewReader(res.Primary))
	if err != nil {
		t.Fatalf("decode config: %v", err)
	}
	if cfg.Width != 800 || cfg.Height != 600 {
		t.Fatalf("unexpected dimensions: %dx%d", cfg.Width, cfg.Height)
	}

	outDir := "testdata/generated"
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		t.Fatalf("mkdir out dir: %v", err)
	}
	outPath := filepath.Join(outDir, "grid_sdr.jpg")
	if err := os.WriteFile(outPath, res.Primary, 0o644); err != nil {
		t.Fatalf("write output: %v", err)
	}
}

func TestGridHDR(t *testing.T) {
	paths := []string{
		"testdata/small_uhdr.jpg",
		"testdata/sample_srgb.jpg",
	}

	readers := make([]io.Reader, 0, len(paths))
	files := make([]*os.File, 0, len(paths))
	for _, p := range paths {
		f, err := os.Open(p)
		if err != nil {
			t.Fatalf("open %s: %v", p, err)
		}
		files = append(files, f)
		readers = append(readers, f)
	}
	for _, f := range files {
		defer f.Close()
	}

	res, err := Grid(readers, 2, 400, 300, &GridOptions{
		Quality:       85,
		Interpolation: InterpolationLanczos2,
	})
	if err != nil {
		t.Fatalf("grid: %v", err)
	}
	if res == nil || res.Container == nil || res.Primary == nil {
		t.Fatalf("missing result")
	}
	if res.Gainmap == nil {
		t.Fatalf("expected gainmap output")
	}
	if _, err := Split(bytes.NewReader(res.Container)); err != nil {
		t.Fatalf("split grid container: %v", err)
	}

	if err := os.WriteFile("testdata/uhdr_grid.jpg", res.Container, 0o644); err != nil {
		t.Fatalf("write container: %v", err)
	}
	if err := os.WriteFile("testdata/uhdr_grid_primary.jpg", res.Primary, 0o644); err != nil {
		t.Fatalf("write primary: %v", err)
	}
	if err := os.WriteFile("testdata/uhdr_grid_gainmap.jpg", res.Gainmap, 0o644); err != nil {
		t.Fatalf("write gainmap: %v", err)
	}
}
