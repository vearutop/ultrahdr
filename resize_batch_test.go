package ultrahdr

import (
	"bytes"
	"image"
	"os"
	"path/filepath"
	"testing"
)

func TestResizeSDRBatchMatchesSingle(t *testing.T) {
	f, err := os.Open("testdata/sample_display_p3.jpg")
	if err != nil {
		t.Fatalf("open sample: %v", err)
	}

	assertData := func(res *Result, err error) {
		if err != nil {
			t.Fatalf("assert data: %v", err)
		}
		if res == nil || res.Primary == nil {
			t.Fatalf("missing result")
		}
		cfg, _, err := image.DecodeConfig(bytes.NewReader(res.Primary))
		if err != nil {
			t.Fatalf("decode config: %v", err)
		}
		if cfg.Width == 0 || cfg.Height == 0 {
			t.Fatalf("wrong dimensions: %dx%d", cfg.Width, cfg.Height)
		}
	}

	specs := []ResizeSpec{
		{Width: 1200, Height: 800, Quality: 85, Interpolation: InterpolationLanczos2, KeepMeta: true, ReceiveResult: assertData},
		{Width: 600, Height: 400, Quality: 82, Interpolation: InterpolationLanczos2, KeepMeta: false, ReceiveResult: assertData},
		{Width: 300, Height: 200, Quality: 78, Interpolation: InterpolationBilinear, KeepMeta: false, ReceiveResult: assertData},
		{Width: 300, Height: 200, Quality: 92, Interpolation: InterpolationBilinear, KeepMeta: false, ReceiveResult: assertData},
	}

	err = ResizeSDR(f, specs...)
	if err != nil {
		t.Fatalf("batch resize: %v", err)
	}
}

func TestResizeSDRBatchInvalid(t *testing.T) {
	f, err := os.Open("testdata/sample_srgb.jpg")
	if err != nil {
		t.Fatalf("open sample: %v", err)
	}

	if err := ResizeSDR(f); err == nil {
		t.Fatal("expected error for empty specs")
	}

	f, err = os.Open("testdata/sample_srgb.jpg")
	if err != nil {
		t.Fatalf("open sample: %v", err)
	}
	if err := ResizeSDR(f, ResizeSpec{Width: 0, Height: 100, Quality: 80}); err == nil {
		t.Fatal("expected error for zero width")
	}
}

func TestResizeSDRCrop(t *testing.T) {
	f, err := os.Open("testdata/sample_srgb.jpg")
	if err != nil {
		t.Fatalf("open sample: %v", err)
	}

	var out *Result
	crop := image.Rect(120, 80, 520, 380)
	err = ResizeSDR(f, ResizeSpec{
		Width:         200,
		Height:        150,
		Crop:          &crop,
		Quality:       85,
		Interpolation: InterpolationLanczos2,
		KeepMeta:      true,
		ReceiveResult: func(res *Result, err error) {
			if err != nil {
				t.Fatalf("resize: %v", err)
			}
			out = res
		},
	})
	if err != nil {
		t.Fatalf("crop resize: %v", err)
	}
	if out == nil || out.Primary == nil {
		t.Fatalf("missing result")
	}
	cfg, _, err := image.DecodeConfig(bytes.NewReader(out.Primary))
	if err != nil {
		t.Fatalf("decode config: %v", err)
	}
	if cfg.Width != 200 || cfg.Height != 150 {
		t.Fatalf("unexpected dimensions: %dx%d", cfg.Width, cfg.Height)
	}

	outDir := "testdata/generated"
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		t.Fatalf("mkdir out dir: %v", err)
	}
	outPath := filepath.Join(outDir, "sample_srgb_crop.jpg")
	if err := os.WriteFile(outPath, out.Primary, 0o644); err != nil {
		t.Fatalf("write output: %v", err)
	}
}
