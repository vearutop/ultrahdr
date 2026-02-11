package ultrahdr

import (
	"image"
	"image/color"
	_ "image/jpeg" // Register JPEG decoder.
)

type rgb struct {
	r, g, b float32
}

func sampleSDR(img image.Image, x, y int) rgb {
	b := img.Bounds()
	if x < b.Min.X {
		x = b.Min.X
	}
	if y < b.Min.Y {
		y = b.Min.Y
	}
	if x >= b.Max.X {
		x = b.Max.X - 1
	}
	if y >= b.Max.Y {
		y = b.Max.Y - 1
	}
	r, g, b2, _ := img.At(x, y).RGBA()
	// RGBA returns 16-bit values in [0, 65535]
	return rgb{
		r: srgbInvOetf(float32(r) / 65535.0),
		g: srgbInvOetf(float32(g) / 65535.0),
		b: srgbInvOetf(float32(b2) / 65535.0),
	}
}

func isGrayImage(img image.Image) bool {
	switch img.(type) {
	case *image.Gray, *image.Gray16:
		return true
	default:
		return false
	}
}

func grayAt(img image.Image, x, y int) uint8 {
	c := color.GrayModel.Convert(img.At(img.Bounds().Min.X+x, img.Bounds().Min.Y+y)).(color.Gray)
	return c.Y
}

func rgbAt(img image.Image, x, y int) (uint8, uint8, uint8) {
	r, g, b, _ := img.At(img.Bounds().Min.X+x, img.Bounds().Min.Y+y).RGBA()
	return uint8(r >> 8), uint8(g >> 8), uint8(b >> 8)
}

func max3(a, b, c float32) float32 {
	if a >= b && a >= c {
		return a
	}
	if b >= a && b >= c {
		return b
	}
	return c
}
