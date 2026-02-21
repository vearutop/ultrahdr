package ultrahdr

import (
	"bytes"
	"errors"
	_ "golang.org/x/image/tiff"
	"image"
)

// DecodeTIFFHDR decodes a TIFF image into a linear HDRImage. It supports
// 8/16-bit integer TIFFs via the standard Go decoder.
func DecodeTIFFHDR(data []byte) (*HDRImage, error) {
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	if w <= 0 || h <= 0 {
		return nil, errors.New("invalid TIFF dimensions")
	}
	out := &HDRImage{
		W:   w,
		H:   h,
		Pix: make([]float32, w*h*3),
	}
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			r, g, b2, _ := img.At(b.Min.X+x, b.Min.Y+y).RGBA()
			i := (y*w + x) * 3
			out.Pix[i] = float32(r) / 65535.0
			out.Pix[i+1] = float32(g) / 65535.0
			out.Pix[i+2] = float32(b2) / 65535.0
		}
	}
	return out, nil
}
