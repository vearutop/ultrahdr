package ultrahdr

import "testing"

func TestRebaseFromEXRFile(t *testing.T) {
	if err := RebaseFromEXRFile("testdata/BrightRings.jpg", "testdata/BrightRings.exr",
		"testdata/BrightRings.uhdr.jpg"); err != nil {
		t.Fatal(err)
	}
}
