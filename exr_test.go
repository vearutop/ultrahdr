package ultrahdr

import "testing"

func TestRebaseUltraHDRFromEXRFile(t *testing.T) {
	if err := RebaseUltraHDRFromEXRFile("testdata/BrightRings.jpg", "testdata/BrightRings.exr",
		"testdata/BrightRings.uhdr.jpg", nil, "", ""); err != nil {
		t.Fatal(err)
	}
}
