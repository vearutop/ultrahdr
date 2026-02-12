package ultrahdr_test

import (
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
	bundle, err := ultrahdr.BuildMetadataBundle(sr.PrimaryJPEG, sr.Segs)
	if err != nil {
		return
	}
	_, _ = ultrahdr.AssembleFromBundle(sr.PrimaryJPEG, sr.GainmapJPEG, bundle)
}

func ExampleResizeUltraHDR() {
	data, err := os.ReadFile(filepath.FromSlash("testdata/uhdr.jpg"))
	if err != nil {
		return
	}
	_, _ = ultrahdr.ResizeUltraHDR(data, 2400, 1600)
}

func ExampleResizeJPEG() {
	data, err := os.ReadFile(filepath.FromSlash("testdata/uhdr_thumb_nearest_primary.jpg"))
	if err != nil {
		return
	}
	_, _ = ultrahdr.ResizeJPEG(data, 800, 600, 85, ultrahdr.InterpolationLanczos2, true)
}
