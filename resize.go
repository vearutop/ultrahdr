package ultrahdr

import (
	"bytes"
	"errors"
	"image"
	"image/color"
	"image/draw"
	"math"
	"os"
	"path/filepath"

	"github.com/vearutop/ultrahdr/internal/jpegx"
)

// ResizeOptions controls the UltraHDR resize behavior.
type ResizeOptions struct {
	BaseQuality    int
	GainmapQuality int
	Resizer        Resizer
}

// ResizeResult contains the resized container and its component JPEGs.
type ResizeResult struct {
	Container []byte
	Primary   []byte
	Gainmap   []byte
}

// ResizeUltraHDR resizes an UltraHDR JPEG container to the requested dimensions.
// It returns the new container and the resized primary/gainmap JPEGs.
func ResizeUltraHDR(data []byte, width, height int, opt *ResizeOptions) (*ResizeResult, error) {
	if width <= 0 || height <= 0 {
		return nil, errors.New("invalid target dimensions")
	}
	primary, gainmap, meta, segs, err := SplitWithSegments(data)
	if err != nil {
		return nil, err
	}
	if segs == nil {
		return nil, errors.New("metadata segments missing")
	}
	baseQ := 85
	gainQ := 75
	if opt != nil {
		if opt.BaseQuality > 0 {
			baseQ = opt.BaseQuality
		}
		if opt.GainmapQuality > 0 {
			gainQ = opt.GainmapQuality
		}
	}
	var resizer Resizer
	if opt != nil {
		resizer = opt.Resizer
	}
	primaryThumb, err := resizeJPEG(primary, width, height, nil, baseQ, resizer)
	if err != nil {
		return nil, err
	}
	gainmapThumb, err := resizeGainmapJPEG(gainmap, width, height, nil, gainQ, meta, resizer)
	if err != nil {
		return nil, err
	}
	exif, icc, err := extractExifAndIcc(primary)
	if err != nil {
		return nil, err
	}
	container, err := assembleContainerVipsLike(primaryThumb, gainmapThumb, exif, icc, segs.SecondaryXMP, segs.SecondaryISO)
	if err != nil {
		return nil, err
	}
	return &ResizeResult{
		Container: container,
		Primary:   primaryThumb,
		Gainmap:   gainmapThumb,
	}, nil
}

// ResizeUltraHDRFile reads an UltraHDR JPEG from inPath, resizes it, and writes
// the container to outPath. If primaryOut or gainmapOut are non-empty, the
// resized component JPEGs are written as well.
func ResizeUltraHDRFile(inPath, outPath string, width, height int, opt *ResizeOptions, primaryOut, gainmapOut string) error {
	data, err := os.ReadFile(filepath.Clean(inPath))
	if err != nil {
		return err
	}
	resized, err := ResizeUltraHDR(data, width, height, opt)
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Clean(outPath), resized.Container, 0644); err != nil {
		return err
	}
	if primaryOut != "" {
		if err := os.WriteFile(filepath.Clean(primaryOut), resized.Primary, 0644); err != nil {
			return err
		}
	}
	if gainmapOut != "" {
		if err := os.WriteFile(filepath.Clean(gainmapOut), resized.Gainmap, 0644); err != nil {
			return err
		}
	}
	return nil
}

// Resizer lets callers provide a custom resize implementation.
// The resizer is expected to preserve linear channel values.
type Resizer interface {
	Resize(img image.Image, w, h int) image.Image
}

func resizeJPEG(jpegData []byte, w, h int, segs []appSegment, quality int, resizer Resizer) ([]byte, error) {
	img, _, err := image.Decode(bytes.NewReader(jpegData))
	if err != nil {
		return nil, err
	}
	var outImg image.Image
	if resizer != nil {
		outImg = resizer.Resize(img, w, h)
	} else {
		switch src := img.(type) {
		case *image.YCbCr:
			outImg = resizeYCbCrNearest(src, w, h)
		case *image.Gray:
			dst := image.NewGray(image.Rect(0, 0, w, h))
			nearestScale(dst, src)
			outImg = dst
		default:
			dst := image.NewRGBA(image.Rect(0, 0, w, h))
			nearestScale(dst, img)
			outImg = dst
		}
	}
	out, err := encodeWithQuality(outImg, quality)
	if err != nil {
		return nil, err
	}
	if len(segs) > 0 {
		return insertAppSegments(out, segs)
	}
	return out, nil
}

func resizeGainmapJPEG(jpegData []byte, w, h int, segs []appSegment, quality int, meta *GainMapMetadata, resizer Resizer) ([]byte, error) {
	img, _, err := image.Decode(bytes.NewReader(jpegData))
	if err != nil {
		return nil, err
	}
	if meta == nil {
		return nil, errors.New("gainmap metadata missing")
	}
	var outImg image.Image
	if resizer != nil {
		outImg, err = resizeGainmapLinear(img, w, h, meta, resizer)
		if err != nil {
			return nil, err
		}
	} else {
		switch src := img.(type) {
		case *image.YCbCr:
			outImg = resizeYCbCrNearest(src, w, h)
		case *image.Gray:
			dst := image.NewGray(image.Rect(0, 0, w, h))
			nearestScale(dst, src)
			outImg = dst
		default:
			dst := image.NewRGBA(image.Rect(0, 0, w, h))
			nearestScale(dst, img)
			outImg = dst
		}
	}
	out, err := encodeWithQuality(outImg, quality)
	if err != nil {
		return nil, err
	}
	if len(segs) > 0 {
		return insertAppSegments(out, segs)
	}
	return out, nil
}

func resizeYCbCrNearest(src *image.YCbCr, w, h int) *image.YCbCr {
	dst := image.NewYCbCr(image.Rect(0, 0, w, h), src.SubsampleRatio)
	sb := src.Bounds()
	sw, sh := sb.Dx(), sb.Dy()
	dw, dh := w, h

	for y := 0; y < dh; y++ {
		sy := sb.Min.Y + y*sh/dh
		for x := 0; x < dw; x++ {
			sx := sb.Min.X + x*sw/dw
			dst.Y[y*dst.YStride+x] = src.Y[(sy-sb.Min.Y)*src.YStride+(sx-sb.Min.X)]
		}
	}

	dstCbW, dstCbH := chromaSize(dst.Rect, dst.SubsampleRatio)
	srcCbW, srcCbH := chromaSize(src.Rect, src.SubsampleRatio)
	for y := 0; y < dstCbH; y++ {
		sy := y * srcCbH / dstCbH
		for x := 0; x < dstCbW; x++ {
			sx := x * srcCbW / dstCbW
			dst.Cb[y*dst.CStride+x] = src.Cb[sy*src.CStride+sx]
			dst.Cr[y*dst.CStride+x] = src.Cr[sy*src.CStride+sx]
		}
	}
	return dst
}

func chromaSize(r image.Rectangle, subsample image.YCbCrSubsampleRatio) (cw, ch int) {
	w, h := r.Dx(), r.Dy()
	switch subsample {
	case image.YCbCrSubsampleRatio444:
		return w, h
	case image.YCbCrSubsampleRatio422:
		return (w + 1) / 2, h
	case image.YCbCrSubsampleRatio420:
		return (w + 1) / 2, (h + 1) / 2
	case image.YCbCrSubsampleRatio440:
		return w, (h + 1) / 2
	default:
		return (w + 1) / 2, (h + 1) / 2
	}
}

func nearestScale(dst draw.Image, src image.Image) {
	sb := src.Bounds()
	db := dst.Bounds()
	sw, sh := sb.Dx(), sb.Dy()
	dw, dh := db.Dx(), db.Dy()
	for y := 0; y < dh; y++ {
		sy := sb.Min.Y + y*sh/dh
		for x := 0; x < dw; x++ {
			sx := sb.Min.X + x*sw/dw
			dst.Set(x, y, src.At(sx, sy))
		}
	}
}

func encodeWithQuality(img image.Image, quality int) ([]byte, error) {
	var buf bytes.Buffer
	opt := &jpegx.EncoderOptions{
		Quality:        quality,
		UseQuantTables: false,
		UseHuffman:     false,
		UseSampling:    true,
		Sampling:       [3]jpegx.SamplingFactor{{H: 2, V: 2}, {H: 1, V: 1}, {H: 1, V: 1}},
		SplitDQT:       true,
		SplitDHT:       true,
	}
	if err := jpegx.EncodeWithTables(&buf, img, opt); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func resizeGainmapLinear(img image.Image, w, h int, meta *GainMapMetadata, resizer Resizer) (image.Image, error) {
	if meta == nil {
		return nil, errors.New("gainmap metadata missing")
	}
	isGray := isGrayImage(img)
	if isGray {
		linear := image.NewGray16(image.Rect(0, 0, img.Bounds().Dx(), img.Bounds().Dy()))
		for y := 0; y < linear.Rect.Dy(); y++ {
			for x := 0; x < linear.Rect.Dx(); x++ {
				g := gainmapDecodeValue(grayAt(img, x, y), meta.Gamma[0])
				linear.SetGray16(x, y, color.Gray16{Y: toGray16(g)})
			}
		}
		resized := resizer.Resize(linear, w, h)
		return encodeGainmapGray(resized, meta.Gamma[0]), nil
	}

	linear := image.NewRGBA64(image.Rect(0, 0, img.Bounds().Dx(), img.Bounds().Dy()))
	for y := 0; y < linear.Rect.Dy(); y++ {
		for x := 0; x < linear.Rect.Dx(); x++ {
			r8, g8, b8 := rgbAt(img, x, y)
			r := gainmapDecodeValue(r8, meta.Gamma[0])
			g := gainmapDecodeValue(g8, meta.Gamma[1])
			b := gainmapDecodeValue(b8, meta.Gamma[2])
			linear.SetRGBA64(x, y, color.RGBA64{
				R: toGray16(r),
				G: toGray16(g),
				B: toGray16(b),
				A: 0xFFFF,
			})
		}
	}
	resized := resizer.Resize(linear, w, h)
	return encodeGainmapRGB(resized, meta.Gamma), nil
}

func gainmapDecodeValue(v uint8, gamma float32) float32 {
	g := float32(v) / 255.0
	if gamma != 1 {
		g = float32(math.Pow(float64(g), float64(1.0/gamma)))
	}
	return clamp01(g)
}

func gainmapEncodeValue(v float32, gamma float32) uint8 {
	g := clamp01(v)
	if gamma != 1 {
		g = float32(math.Pow(float64(g), float64(gamma)))
	}
	val := g * 255.0
	if val < 0 {
		val = 0
	}
	if val > 255 {
		val = 255
	}
	return uint8(val + 0.5)
}

func toGray16(v float32) uint16 {
	return uint16(clamp01(v) * 65535.0)
}

func encodeGainmapGray(img image.Image, gamma float32) image.Image {
	b := img.Bounds()
	out := image.NewGray(b)
	for y := 0; y < b.Dy(); y++ {
		for x := 0; x < b.Dx(); x++ {
			c := color.Gray16Model.Convert(img.At(b.Min.X+x, b.Min.Y+y)).(color.Gray16)
			g := float32(c.Y) / 65535.0
			out.SetGray(x, y, color.Gray{Y: gainmapEncodeValue(g, gamma)})
		}
	}
	return out
}

func encodeGainmapRGB(img image.Image, gamma [3]float32) image.Image {
	b := img.Bounds()
	out := image.NewRGBA(b)
	for y := 0; y < b.Dy(); y++ {
		for x := 0; x < b.Dx(); x++ {
			c := color.RGBA64Model.Convert(img.At(b.Min.X+x, b.Min.Y+y)).(color.RGBA64)
			r := float32(c.R) / 65535.0
			g := float32(c.G) / 65535.0
			bv := float32(c.B) / 65535.0
			out.SetRGBA(x, y, color.RGBA{
				R: gainmapEncodeValue(r, gamma[0]),
				G: gainmapEncodeValue(g, gamma[1]),
				B: gainmapEncodeValue(bv, gamma[2]),
				A: 0xFF,
			})
		}
	}
	return out
}

func clamp01(v float32) float32 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}
