package ultrahdr

import (
	"bytes"
	"image"
	"os"
	"testing"
)

func TestResizeHDRCrop(t *testing.T) {
	data, err := os.ReadFile("testdata/small_uhdr.jpg")
	if err != nil {
		t.Fatalf("read sample: %v", err)
	}

	split, err := Split(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("split: %v", err)
	}
	primaryCfg, _, err := image.DecodeConfig(bytes.NewReader(split.Primary))
	if err != nil {
		t.Fatalf("decode primary: %v", err)
	}
	if primaryCfg.Width < 4 || primaryCfg.Height < 4 {
		t.Skip("image too small for crop test")
	}

	marginX := primaryCfg.Width / 10
	marginY := primaryCfg.Height / 10
	if marginX < 1 {
		marginX = 1
	}
	if marginY < 1 {
		marginY = 1
	}
	crop := image.Rect(marginX, marginY, primaryCfg.Width-marginX, primaryCfg.Height-marginY)
	if crop.Dx() <= 0 || crop.Dy() <= 0 {
		t.Skip("invalid crop rect")
	}

	targetW := uint(crop.Dx() / 2)
	targetH := uint(crop.Dy() / 2)
	if targetW == 0 || targetH == 0 {
		t.Skip("invalid target dimensions")
	}

	var out *Result
	err = ResizeHDR(bytes.NewReader(data), ResizeSpec{
		Width:          targetW,
		Height:         targetH,
		Crop:           &crop,
		Quality:        85,
		GainmapQuality: 75,
		Interpolation:  InterpolationLanczos2,
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
	if out == nil || out.Primary == nil || out.Gainmap == nil || out.Container == nil {
		t.Fatalf("missing result")
	}

	cfg, _, err := image.DecodeConfig(bytes.NewReader(out.Primary))
	if err != nil {
		t.Fatalf("decode primary config: %v", err)
	}
	if cfg.Width != int(targetW) || cfg.Height != int(targetH) {
		t.Fatalf("primary dims mismatch: got %dx%d want %dx%d", cfg.Width, cfg.Height, targetW, targetH)
	}
	cfg, _, err = image.DecodeConfig(bytes.NewReader(out.Gainmap))
	if err != nil {
		t.Fatalf("decode gainmap config: %v", err)
	}
	if cfg.Width != int(targetW) || cfg.Height != int(targetH) {
		t.Fatalf("gainmap dims mismatch: got %dx%d want %dx%d", cfg.Width, cfg.Height, targetW, targetH)
	}

	if err := os.WriteFile("testdata/uhdr_crop.jpg", out.Container, 0o644); err != nil {
		t.Fatalf("write container: %v", err)
	}
	if err := os.WriteFile("testdata/uhdr_crop_primary.jpg", out.Primary, 0o644); err != nil {
		t.Fatalf("write primary: %v", err)
	}
	if err := os.WriteFile("testdata/uhdr_crop_gainmap.jpg", out.Gainmap, 0o644); err != nil {
		t.Fatalf("write gainmap: %v", err)
	}
}
