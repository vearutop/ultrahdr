package ultrahdr

import (
	"errors"
	"image"
	"image/draw"
	"math"
)

func resolveCropRects(rect image.Rectangle, primaryBounds, gainmapBounds image.Rectangle) (image.Rectangle, image.Rectangle, error) {
	if rect.Empty() {
		return image.Rectangle{}, image.Rectangle{}, errors.New("invalid crop rectangle")
	}
	if !rect.In(primaryBounds) {
		return image.Rectangle{}, image.Rectangle{}, errors.New("crop rectangle out of bounds")
	}
	if primaryBounds.Dx() == gainmapBounds.Dx() && primaryBounds.Dy() == gainmapBounds.Dy() {
		if primaryBounds.Min == gainmapBounds.Min {
			return rect, rect, nil
		}
		dx := gainmapBounds.Min.X - primaryBounds.Min.X
		dy := gainmapBounds.Min.Y - primaryBounds.Min.Y
		gainRect := image.Rect(rect.Min.X+dx, rect.Min.Y+dy, rect.Max.X+dx, rect.Max.Y+dy)
		if !gainRect.In(gainmapBounds) {
			return image.Rectangle{}, image.Rectangle{}, errors.New("gainmap crop rectangle out of bounds")
		}
		return rect, gainRect, nil
	}
	if primaryBounds.Dx() <= 0 || primaryBounds.Dy() <= 0 {
		return image.Rectangle{}, image.Rectangle{}, errors.New("invalid source dimensions")
	}

	scaleX := float64(gainmapBounds.Dx()) / float64(primaryBounds.Dx())
	scaleY := float64(gainmapBounds.Dy()) / float64(primaryBounds.Dy())

	minX := gainmapBounds.Min.X + int(math.Round(float64(rect.Min.X-primaryBounds.Min.X)*scaleX))
	maxX := gainmapBounds.Min.X + int(math.Round(float64(rect.Max.X-primaryBounds.Min.X)*scaleX))
	minY := gainmapBounds.Min.Y + int(math.Round(float64(rect.Min.Y-primaryBounds.Min.Y)*scaleY))
	maxY := gainmapBounds.Min.Y + int(math.Round(float64(rect.Max.Y-primaryBounds.Min.Y)*scaleY))

	gainRect := image.Rect(minX, minY, maxX, maxY)
	if gainRect.Min.X < gainmapBounds.Min.X {
		gainRect.Min.X = gainmapBounds.Min.X
	}
	if gainRect.Min.Y < gainmapBounds.Min.Y {
		gainRect.Min.Y = gainmapBounds.Min.Y
	}
	if gainRect.Max.X > gainmapBounds.Max.X {
		gainRect.Max.X = gainmapBounds.Max.X
	}
	if gainRect.Max.Y > gainmapBounds.Max.Y {
		gainRect.Max.Y = gainmapBounds.Max.Y
	}
	if gainRect.Empty() {
		return image.Rectangle{}, image.Rectangle{}, errors.New("scaled gainmap crop rectangle invalid")
	}
	return rect, gainRect, nil
}

func validateCropRect(rect, bounds image.Rectangle) error {
	if rect.Empty() {
		return errors.New("invalid crop rectangle")
	}
	if !rect.In(bounds) {
		return errors.New("crop rectangle out of bounds")
	}
	return nil
}

func cropImage(img image.Image, rect image.Rectangle) (image.Image, error) {
	if img == nil {
		return nil, errors.New("missing image")
	}
	if err := validateCropRect(rect, img.Bounds()); err != nil {
		return nil, err
	}
	if rect.Min == img.Bounds().Min && rect.Max == img.Bounds().Max {
		return img, nil
	}

	switch src := img.(type) {
	case *image.YCbCr:
		return cropYCbCr(src, rect), nil
	case *image.Gray:
		return cropGray(src, rect), nil
	case *image.Gray16:
		return cropGray16(src, rect), nil
	case *image.RGBA:
		return cropRGBA(src, rect), nil
	case *image.NRGBA:
		return cropNRGBA(src, rect), nil
	case *image.RGBA64:
		return cropRGBA64(src, rect), nil
	case *image.NRGBA64:
		return cropNRGBA64(src, rect), nil
	default:
		dst := image.NewRGBA(image.Rect(0, 0, rect.Dx(), rect.Dy()))
		draw.Draw(dst, dst.Bounds(), img, rect.Min, draw.Src)
		return dst, nil
	}
}

func cropYCbCr(src *image.YCbCr, rect image.Rectangle) *image.YCbCr {
	r := rect.Intersect(src.Bounds())
	dst := image.NewYCbCr(image.Rect(0, 0, r.Dx(), r.Dy()), src.SubsampleRatio)

	for y := 0; y < r.Dy(); y++ {
		srcOff := (r.Min.Y-src.Rect.Min.Y+y)*src.YStride + (r.Min.X - src.Rect.Min.X)
		dstOff := y * dst.YStride
		copy(dst.Y[dstOff:dstOff+r.Dx()], src.Y[srcOff:srcOff+r.Dx()])
	}

	sx, sy := subsampleFactors(src.SubsampleRatio)
	srcCbStartX := (r.Min.X - src.Rect.Min.X) / sx
	srcCbStartY := (r.Min.Y - src.Rect.Min.Y) / sy
	cw := (r.Dx() + sx - 1) / sx
	ch := (r.Dy() + sy - 1) / sy
	for y := 0; y < ch; y++ {
		srcOff := (srcCbStartY+y)*src.CStride + srcCbStartX
		dstOff := y * dst.CStride
		copy(dst.Cb[dstOff:dstOff+cw], src.Cb[srcOff:srcOff+cw])
		copy(dst.Cr[dstOff:dstOff+cw], src.Cr[srcOff:srcOff+cw])
	}
	return dst
}

func cropGray(src *image.Gray, rect image.Rectangle) *image.Gray {
	r := rect.Intersect(src.Bounds())
	dst := image.NewGray(image.Rect(0, 0, r.Dx(), r.Dy()))
	for y := 0; y < r.Dy(); y++ {
		srcOff := (r.Min.Y-src.Rect.Min.Y+y)*src.Stride + (r.Min.X - src.Rect.Min.X)
		dstOff := y * dst.Stride
		copy(dst.Pix[dstOff:dstOff+r.Dx()], src.Pix[srcOff:srcOff+r.Dx()])
	}
	return dst
}

func cropGray16(src *image.Gray16, rect image.Rectangle) *image.Gray16 {
	r := rect.Intersect(src.Bounds())
	dst := image.NewGray16(image.Rect(0, 0, r.Dx(), r.Dy()))
	rowBytes := r.Dx() * 2
	for y := 0; y < r.Dy(); y++ {
		srcOff := (r.Min.Y-src.Rect.Min.Y+y)*src.Stride + (r.Min.X-src.Rect.Min.X)*2
		dstOff := y * dst.Stride
		copy(dst.Pix[dstOff:dstOff+rowBytes], src.Pix[srcOff:srcOff+rowBytes])
	}
	return dst
}

func cropRGBA(src *image.RGBA, rect image.Rectangle) *image.RGBA {
	r := rect.Intersect(src.Bounds())
	dst := image.NewRGBA(image.Rect(0, 0, r.Dx(), r.Dy()))
	rowBytes := r.Dx() * 4
	for y := 0; y < r.Dy(); y++ {
		srcOff := (r.Min.Y-src.Rect.Min.Y+y)*src.Stride + (r.Min.X-src.Rect.Min.X)*4
		dstOff := y * dst.Stride
		copy(dst.Pix[dstOff:dstOff+rowBytes], src.Pix[srcOff:srcOff+rowBytes])
	}
	return dst
}

func cropNRGBA(src *image.NRGBA, rect image.Rectangle) *image.NRGBA {
	r := rect.Intersect(src.Bounds())
	dst := image.NewNRGBA(image.Rect(0, 0, r.Dx(), r.Dy()))
	rowBytes := r.Dx() * 4
	for y := 0; y < r.Dy(); y++ {
		srcOff := (r.Min.Y-src.Rect.Min.Y+y)*src.Stride + (r.Min.X-src.Rect.Min.X)*4
		dstOff := y * dst.Stride
		copy(dst.Pix[dstOff:dstOff+rowBytes], src.Pix[srcOff:srcOff+rowBytes])
	}
	return dst
}

func cropRGBA64(src *image.RGBA64, rect image.Rectangle) *image.RGBA64 {
	r := rect.Intersect(src.Bounds())
	dst := image.NewRGBA64(image.Rect(0, 0, r.Dx(), r.Dy()))
	rowBytes := r.Dx() * 8
	for y := 0; y < r.Dy(); y++ {
		srcOff := (r.Min.Y-src.Rect.Min.Y+y)*src.Stride + (r.Min.X-src.Rect.Min.X)*8
		dstOff := y * dst.Stride
		copy(dst.Pix[dstOff:dstOff+rowBytes], src.Pix[srcOff:srcOff+rowBytes])
	}
	return dst
}

func cropNRGBA64(src *image.NRGBA64, rect image.Rectangle) *image.NRGBA64 {
	r := rect.Intersect(src.Bounds())
	dst := image.NewNRGBA64(image.Rect(0, 0, r.Dx(), r.Dy()))
	rowBytes := r.Dx() * 8
	for y := 0; y < r.Dy(); y++ {
		srcOff := (r.Min.Y-src.Rect.Min.Y+y)*src.Stride + (r.Min.X-src.Rect.Min.X)*8
		dstOff := y * dst.Stride
		copy(dst.Pix[dstOff:dstOff+rowBytes], src.Pix[srcOff:srcOff+rowBytes])
	}
	return dst
}

func subsampleFactors(r image.YCbCrSubsampleRatio) (sx, sy int) {
	switch r {
	case image.YCbCrSubsampleRatio444:
		return 1, 1
	case image.YCbCrSubsampleRatio422:
		return 2, 1
	case image.YCbCrSubsampleRatio420:
		return 2, 2
	case image.YCbCrSubsampleRatio440:
		return 1, 2
	case image.YCbCrSubsampleRatio411:
		return 4, 1
	case image.YCbCrSubsampleRatio410:
		return 4, 2
	default:
		return 2, 2
	}
}
