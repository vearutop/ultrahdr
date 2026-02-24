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

func TestGridReceivePosition(t *testing.T) {
	paths := []string{
		"testdata/sample_srgb.jpg",
		"testdata/sample_display_p3.jpg",
		"testdata/sample_adobe_rgb.jpg",
	}
	const (
		cols  = 2
		cellW = 400
		cellH = 300
	)

	type pos struct {
		i      int
		top    uint
		left   uint
		width  uint
		height uint
	}

	readers := make([]io.Reader, 0, len(paths))
	decoded := make([]image.Image, 0, len(paths))
	for _, p := range paths {
		data, err := os.ReadFile(p)
		if err != nil {
			t.Fatalf("read %s: %v", p, err)
		}
		img, _, err := image.Decode(bytes.NewReader(data))
		if err != nil {
			t.Fatalf("decode %s: %v", p, err)
		}
		readers = append(readers, bytes.NewReader(data))
		decoded = append(decoded, img)
	}

	got := make([]pos, 0, len(paths))
	_, err := Grid(readers, cols, cellW, cellH, &GridOptions{
		ReceivePosition: func(i int, top, left uint, width, height uint) {
			got = append(got, pos{
				i:      i,
				top:    top,
				left:   left,
				width:  width,
				height: height,
			})
		},
	})
	if err != nil {
		t.Fatalf("grid: %v", err)
	}

	if len(got) != len(paths) {
		t.Fatalf("unexpected positions count: got %d want %d", len(got), len(paths))
	}

	for i := range decoded {
		_, w, h := resizeToFit(decoded[i], cellW, cellH, InterpolationLanczos2)
		col := i % cols
		row := i / cols
		wantLeft := uint(col*cellW + (cellW-w)/2)
		wantTop := uint(row*cellH + (cellH-h)/2)
		wantWidth := uint(w)
		wantHeight := uint(h)

		if got[i].i != i {
			t.Fatalf("position[%d] unexpected index: got %d want %d", i, got[i].i, i)
		}
		if got[i].top != wantTop || got[i].left != wantLeft || got[i].width != wantWidth || got[i].height != wantHeight {
			t.Fatalf(
				"position[%d] mismatch: got (top=%d left=%d width=%d height=%d), want (top=%d left=%d width=%d height=%d)",
				i,
				got[i].top, got[i].left, got[i].width, got[i].height,
				wantTop, wantLeft, wantWidth, wantHeight,
			)
		}
	}
}
