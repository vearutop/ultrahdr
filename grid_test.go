package ultrahdr

import (
	"bytes"
	"image"
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
