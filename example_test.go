package ultrahdr_test

import (
	"bytes"
	"io"
	"os"
	"path/filepath"

	"github.com/vearutop/ultrahdr"
)

func ExampleIsUltraHDR() {
	f, err := os.Open(filepath.FromSlash("testdata/uhdr.jpg"))
	if err != nil {
		return
	}
	defer f.Close()

	_, _ = ultrahdr.IsUltraHDR(f)
}

func ExampleSplit_joinWithBundle() {
	data, err := os.ReadFile(filepath.FromSlash("testdata/uhdr.jpg"))
	if err != nil {
		return
	}
	sr, err := ultrahdr.Split(data)
	if err != nil {
		return
	}
	bundle, err := ultrahdr.BuildMetadataBundle(sr.Primary, sr.Segs)
	if err != nil {
		return
	}
	_, _ = ultrahdr.AssembleFromBundle(sr.Primary, sr.Gainmap, bundle)
}

func ExampleResizeUltraHDR() {
	data, err := os.ReadFile(filepath.FromSlash("testdata/uhdr.jpg"))
	if err != nil {
		return
	}
	_, _ = ultrahdr.ResizeUltraHDR(data, 2400, 1600)
}

func ExampleResizeSDR() {
	data, err := os.ReadFile(filepath.FromSlash("testdata/sample_srgb.jpg"))
	if err != nil {
		return
	}
	_, _ = ultrahdr.ResizeSDR(io.NopCloser(bytes.NewReader(data)), 800, 600, 85, ultrahdr.InterpolationLanczos2, true)
}

func ExampleResizeSDRBatch() {
	data, err := os.ReadFile(filepath.FromSlash("testdata/sample_display_p3.jpg"))
	if err != nil {
		return
	}
	specs := []ultrahdr.ResizeSpec{
		{Width: 1200, Height: 800, Quality: 85, Interpolation: ultrahdr.InterpolationLanczos2, KeepMeta: true, ReceiveResult: func(d []byte, err error) { _ = d }},
		{Width: 600, Height: 400, Quality: 82, Interpolation: ultrahdr.InterpolationLanczos2, KeepMeta: false, ReceiveResult: func(d []byte, err error) { _ = d }},
		{Width: 300, Height: 200, Quality: 78, Interpolation: ultrahdr.InterpolationLanczos2, KeepMeta: false, ReceiveResult: func(d []byte, err error) { _ = d }},
	}
	err = ultrahdr.ResizeSDRBatch(io.NopCloser(bytes.NewReader(data)), specs)
	if err != nil {
		return
	}
}
