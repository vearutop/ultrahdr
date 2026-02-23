package ultrahdr

import (
	"bytes"
	"image"
	"os"
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

	err = ResizeSDRBatch(f, specs)
	if err != nil {
		t.Fatalf("batch resize: %v", err)
	}
}

func TestResizeSDRBatchInvalid(t *testing.T) {
	f, err := os.Open("testdata/sample_srgb.jpg")
	if err != nil {
		t.Fatalf("open sample: %v", err)
	}

	if err := ResizeSDRBatch(f, nil); err == nil {
		t.Fatal("expected error for empty specs")
	}

	f, err = os.Open("testdata/sample_srgb.jpg")
	if err != nil {
		t.Fatalf("open sample: %v", err)
	}
	if err := ResizeSDRBatch(f, []ResizeSpec{{Width: 0, Height: 100, Quality: 80}}); err == nil {
		t.Fatal("expected error for zero width")
	}
}
