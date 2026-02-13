package ultrahdr

import (
	"image"
	"image/color"
	_ "image/jpeg" // Register JPEG decoder.
)

type rgb struct {
	r, g, b float32
}

func sampleSDRInProfile(img image.Image, x, y int, src colorProfile, dstGamut colorGamut) rgb {
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
	v := rgb{
		r: invOETF(float32(r)/65535.0, src.transfer),
		g: invOETF(float32(g)/65535.0, src.transfer),
		b: invOETF(float32(b2)/65535.0, src.transfer),
	}
	return convertLinearGamut(v, src.gamut, dstGamut)
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
