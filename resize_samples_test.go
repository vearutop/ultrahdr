package ultrahdr

import (
	"bytes"
	"image"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestResizeJPEGSampleColorSpaces(t *testing.T) {
	samples, err := filepath.Glob(filepath.FromSlash("testdata/sample_*.jpg"))
	if err != nil {
		t.Fatalf("glob samples: %v", err)
	}
	if len(samples) == 0 {
		t.Skip("no sample_* files found")
	}

	outDir := filepath.FromSlash("testdata/generated")
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		t.Fatalf("mkdir out dir: %v", err)
	}

	const (
		outW    = 600
		outH    = 400
		quality = 85
	)

	for _, sample := range samples {
		sample := sample
		t.Run(filepath.Base(sample), func(t *testing.T) {
			data, err := os.ReadFile(sample)
			if err != nil {
				t.Fatalf("read sample: %v", err)
			}

			withoutMeta, err := ResizeJPEG(data, outW, outH, quality, InterpolationLanczos2, false)
			if err != nil {
				t.Fatalf("resize without meta: %v", err)
			}
			withMeta, err := ResizeJPEG(data, outW, outH, quality, InterpolationLanczos2, true)
			if err != nil {
				t.Fatalf("resize with meta: %v", err)
			}

			checkDims := func(label string, b []byte) {
				cfg, _, err := image.DecodeConfig(bytes.NewReader(b))
				if err != nil {
					t.Fatalf("decode config %s: %v", label, err)
				}
				if cfg.Width != outW || cfg.Height != outH {
					t.Fatalf("%s dims mismatch: got %dx%d want %dx%d", label, cfg.Width, cfg.Height, outW, outH)
				}
			}
			checkDims("without_meta", withoutMeta)
			checkDims("with_meta", withMeta)

			base := strings.TrimSuffix(filepath.Base(sample), filepath.Ext(sample))
			noMetaPath := filepath.Join(outDir, base+"_th_nometa.jpg")
			withMetaPath := filepath.Join(outDir, base+"_th_keepmeta.jpg")

			if err := os.WriteFile(noMetaPath, withoutMeta, 0o644); err != nil {
				t.Fatalf("write nometa output: %v", err)
			}
			if err := os.WriteFile(withMetaPath, withMeta, 0o644); err != nil {
				t.Fatalf("write keepmeta output: %v", err)
			}
		})
	}
}
