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
	primary, gainmap, _, segs, err := ultrahdr.SplitWithSegments(data)
	if err != nil {
		return
	}
	bundle, err := ultrahdr.BuildMetadataBundle(primary, segs)
	if err != nil {
		return
	}
	_, _ = ultrahdr.AssembleFromBundle(primary, gainmap, bundle)
}

func ExampleResizeUltraHDR() {
	data, err := os.ReadFile(filepath.FromSlash("testdata/uhdr.jpg"))
	if err != nil {
		return
	}
	_, _ = ultrahdr.ResizeUltraHDR(data, 2400, 1600, &ultrahdr.ResizeOptions{
		BaseQuality:    85,
		GainmapQuality: 75,
	})
}
