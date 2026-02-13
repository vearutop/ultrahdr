package ultrahdr

import (
	"bytes"
	"image"
	"os"
	"testing"
)

func TestResizeJPEGBatchMatchesSingle(t *testing.T) {
	data, err := os.ReadFile("testdata/sample_display_p3.jpg")
	if err != nil {
		t.Fatalf("read sample: %v", err)
	}

	specs := []ResizeJPEGSpec{
		{Width: 1200, Height: 800, Quality: 85, Interpolation: InterpolationLanczos2, KeepMeta: true},
		{Width: 600, Height: 400, Quality: 82, Interpolation: InterpolationLanczos2, KeepMeta: false},
		{Width: 300, Height: 200, Quality: 78, Interpolation: InterpolationBilinear, KeepMeta: false},
		{Width: 300, Height: 200, Quality: 92, Interpolation: InterpolationBilinear, KeepMeta: false},
	}

	batch, err := ResizeJPEGBatch(data, specs)
	if err != nil {
		t.Fatalf("batch resize: %v", err)
	}
	if len(batch) != len(specs) {
		t.Fatalf("unexpected outputs: got %d want %d", len(batch), len(specs))
	}

	for i, s := range specs {
		if batch[i].Spec != s {
			t.Fatalf("spec mismatch at index %d", i)
		}
		single, err := ResizeJPEG(data, s.Width, s.Height, s.Quality, s.Interpolation, s.KeepMeta)
		if err != nil {
			t.Fatalf("single resize %d: %v", i, err)
		}
		if !bytes.Equal(batch[i].Data, single) {
			t.Fatalf("output mismatch at index %d", i)
		}

		cfg, _, err := image.DecodeConfig(bytes.NewReader(batch[i].Data))
		if err != nil {
			t.Fatalf("decode config %d: %v", i, err)
		}
		if cfg.Width != int(s.Width) || cfg.Height != int(s.Height) {
			t.Fatalf("dims mismatch at index %d: got %dx%d want %dx%d", i, cfg.Width, cfg.Height, s.Width, s.Height)
		}
	}
}

func TestResizeJPEGBatchInvalid(t *testing.T) {
	data, err := os.ReadFile("testdata/sample_srgb.jpg")
	if err != nil {
		t.Fatalf("read sample: %v", err)
	}

	if _, err := ResizeJPEGBatch(data, nil); err == nil {
		t.Fatal("expected error for empty specs")
	}

	if _, err := ResizeJPEGBatch(data, []ResizeJPEGSpec{{Width: 0, Height: 100, Quality: 80}}); err == nil {
		t.Fatal("expected error for zero width")
	}
}
