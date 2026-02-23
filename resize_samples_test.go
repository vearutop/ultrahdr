package ultrahdr

import (
	"bytes"
	"image"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestResizeSDRSampleColorSpaces(t *testing.T) {
	samples, err := filepath.Glob("testdata/sample_*.jpg")
	if err != nil {
		t.Fatalf("glob samples: %v", err)
	}
	if len(samples) == 0 {
		t.Skip("no sample_* files found")
	}

	outDir := "testdata/generated"
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
			f, err := os.Open(sample)
			if err != nil {
				t.Fatalf("open sample: %v", err)
			}
			withoutMeta, err := ResizeSDR(f, outW, outH, quality, InterpolationLanczos2, false)
			if err != nil {
				t.Fatalf("resize without meta: %v", err)
			}
			f, err = os.Open(sample)
			if err != nil {
				t.Fatalf("open sample: %v", err)
			}
			withMeta, err := ResizeSDR(f, outW, outH, quality, InterpolationLanczos2, true)
			if err != nil {
				t.Fatalf("resize with meta: %v", err)
			}

			checkDims := func(label string, res *Result) {
				if res == nil || res.Primary == nil {
					t.Fatalf("%s missing result", label)
				}
				cfg, _, err := image.DecodeConfig(bytes.NewReader(res.Primary))
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

			if err := os.WriteFile(noMetaPath, withoutMeta.Primary, 0o644); err != nil {
				t.Fatalf("write nometa output: %v", err)
			}
			if err := os.WriteFile(withMetaPath, withMeta.Primary, 0o644); err != nil {
				t.Fatalf("write keepmeta output: %v", err)
			}
		})
	}
}
