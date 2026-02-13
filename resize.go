package ultrahdr

import (
	"bytes"
	"errors"
	"fmt"
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
	PrimaryQuality int
	GainmapQuality int
	// Interpolation selects the built-in interpolation mode for the primary image and gainmap
	// when Resize is nil.
	Interpolation Interpolation
	OnResult      func(res *ResizeResult)
	OnSplit       func(sr *SplitResult)
	PrimaryOut    string
	GainmapOut    string
}

// ResizeResult contains the resized container and its component JPEGs.
type ResizeResult struct {
	Container []byte
	Primary   []byte
	Gainmap   []byte
}

// ResizeUltraHDR resizes an UltraHDR JPEG container to the requested dimensions.
// It returns the new container and the resized primary/gainmap JPEGs.
func ResizeUltraHDR(data []byte, width, height uint, opts ...func(o *ResizeOptions)) (*ResizeResult, error) {
	if width <= 0 || height <= 0 {
		return nil, errors.New("invalid target dimensions")
	}
	sr, err := Split(data)
	if err != nil {
		return nil, fmt.Errorf("split: %w", err)
	}
	if sr.Segs == nil {
		return nil, errors.New("metadata segments missing")
	}

	opt := ResizeOptions{
		PrimaryQuality: 85,
		GainmapQuality: 75,
		Interpolation:  InterpolationNearest,
	}

	for _, applyOpt := range opts {
		applyOpt(&opt)
	}

	if opt.OnSplit != nil {
		opt.OnSplit(sr)
	}

	primaryThumb, err := resizeJPEG(sr.PrimaryJPEG, width, height, nil, opt.PrimaryQuality, opt.Interpolation)
	if err != nil {
		return nil, fmt.Errorf("resize primary: %w", err)
	}
	gainmapThumb, err := resizeGainmapJPEG(sr.GainmapJPEG, width, height, nil, opt.GainmapQuality, sr.Meta, opt.Interpolation)
	if err != nil {
		return nil, fmt.Errorf("resize gainmap: %w", err)
	}
	exif, icc, err := extractExifAndIcc(sr.PrimaryJPEG)
	if err != nil {
		return nil, fmt.Errorf("extract exif and icc: %w", err)
	}
	secondaryISO := sr.Segs.SecondaryISO
	if len(secondaryISO) == 0 && sr.Meta != nil {
		secondaryISO, err = buildIsoPayload(sr.Meta)
		if err != nil {
			return nil, fmt.Errorf("encode gainmap iso: %w", err)
		}
	}
	container, err := assembleContainerVipsLike(primaryThumb, gainmapThumb, exif, icc, sr.Segs.SecondaryXMP, secondaryISO)
	if err != nil {
		return nil, fmt.Errorf("assemble container: %w", err)
	}

	res := ResizeResult{
		Container: container,
		Primary:   primaryThumb,
		Gainmap:   gainmapThumb,
	}

	if opt.OnResult != nil {
		opt.OnResult(&res)
	}

	return &res, nil
}

// ResizeJPEG resizes a regular JPEG to the requested dimensions using the built-in
// interpolation. When keepMeta is true, EXIF and ICC segments are preserved.
// When keepMeta is false and input is Display P3, output pixels are converted to sRGB.
func ResizeJPEG(data []byte, width, height uint, quality int, interp Interpolation, keepMeta bool) ([]byte, error) {
	if width <= 0 || height <= 0 {
		return nil, errors.New("invalid target dimensions")
	}
	srcGamut := colorGamutSRGB
	if _, icc, err := extractExifAndIcc(data); err == nil {
		srcGamut = detectGamutFromICCProfile(collectICCProfile(icc))
	}
	dstGamut := srcGamut

	var segs []appSegment
	if keepMeta {
		exif, icc, err := extractExifAndIcc(data)
		if err != nil {
			return nil, err
		}
		if exif != nil {
			segs = append(segs, appSegment{marker: markerAPP1, payload: exif})
		}
		for _, seg := range icc {
			segs = append(segs, appSegment{marker: markerAPP2, payload: seg})
		}
	} else {
		// Browsers often assume sRGB when profile is missing.
		dstGamut = colorGamutSRGB
	}
	return resizeJPEGWithGamut(data, width, height, segs, quality, interp, srcGamut, dstGamut)
}

// ResizeUltraHDRFile reads an UltraHDR JPEG from inPath, resizes it, and writes
// the container to outPath. If ResizeOptions.PrimaryOut or ResizeOptions.GainmapOut are non-empty, the
// resized component JPEGs are written as well.
func ResizeUltraHDRFile(inPath, outPath string, width, height uint, opts ...func(opt *ResizeOptions)) error {
	data, err := os.ReadFile(inPath)
	if err != nil {
		return err
	}
	resized, err := ResizeUltraHDR(data, width, height, opts...)
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Clean(outPath), resized.Container, 0o644); err != nil {
		return err
	}

	opt := ResizeOptions{}
	for _, applyOpt := range opts {
		applyOpt(&opt)
	}

	if opt.PrimaryOut != "" {
		if err := os.WriteFile(opt.PrimaryOut, resized.Primary, 0o644); err != nil {
			return fmt.Errorf("write primary: %w", err)
		}
	}
	if opt.GainmapOut != "" {
		if err := os.WriteFile(opt.GainmapOut, resized.Gainmap, 0o644); err != nil {
			return fmt.Errorf("write gainmap: %w", err)
		}
	}
	return nil
}

// Interpolation selects the built-in interpolation mode.
type Interpolation int

const (
	// InterpolationNearest is nearest-neighbor sampling.
	InterpolationNearest Interpolation = iota
	// InterpolationBilinear is linear sampling.
	InterpolationBilinear
	// InterpolationBicubic is cubic sampling.
	InterpolationBicubic
	// InterpolationMitchellNetravali is Mitchell-Netravali sampling.
	InterpolationMitchellNetravali
	// InterpolationLanczos2 is Lanczos sampling with a=2.
	InterpolationLanczos2
	// InterpolationLanczos3 is Lanczos sampling with a=3.
	InterpolationLanczos3
)

func resizeJPEG(jpegData []byte, w, h uint, segs []appSegment, quality int, interp Interpolation) ([]byte, error) {
	return resizeJPEGWithGamut(jpegData, w, h, segs, quality, interp, colorGamutSRGB, colorGamutSRGB)
}

func resizeJPEGWithGamut(jpegData []byte, w, h uint, segs []appSegment, quality int, interp Interpolation, srcGamut, dstGamut colorGamut) ([]byte, error) {
	img, _, err := image.Decode(bytes.NewReader(jpegData))
	if err != nil {
		return nil, err
	}
	outImg := resizeImageInterpolated(img, int(w), int(h), interp)
	if srcGamut != dstGamut {
		outImg = convertImageGamut(outImg, srcGamut, dstGamut)
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

func resizeGainmapJPEG(jpegData []byte, w, h uint, segs []appSegment, quality int, meta *GainMapMetadata, interp Interpolation) ([]byte, error) {
	img, _, err := image.Decode(bytes.NewReader(jpegData))
	if err != nil {
		return nil, err
	}
	if meta == nil {
		return nil, errors.New("gainmap metadata missing")
	}
	outImg := resizeImageInterpolated(img, int(w), int(h), interp)
	out, err := encodeWithQuality(outImg, quality)
	if err != nil {
		return nil, err
	}
	if len(segs) > 0 {
		return insertAppSegments(out, segs)
	}
	return out, nil
}

func resizeImageInterpolated(img image.Image, w, h int, interp Interpolation) image.Image {
	switch src := img.(type) {
	case *image.YCbCr:
		return resizeYCbCrInterpolated(src, w, h, interp)
	case *image.Gray:
		return resizeGrayInterpolated(src, w, h, interp)
	case *image.Gray16:
		return resizeGray16Interpolated(src, w, h, interp)
	case *image.RGBA:
		return resizeRGBAInterpolated(src, w, h, interp)
	case *image.NRGBA:
		return resizeNRGBAInterpolated(src, w, h, interp)
	case *image.RGBA64:
		return resizeRGBA64Interpolated(src, w, h, interp)
	case *image.NRGBA64:
		return resizeNRGBA64Interpolated(src, w, h, interp)
	default:
		dst := image.NewRGBA(image.Rect(0, 0, w, h))
		nearestScale(dst, img)
		return dst
	}
}

func convertImageGamut(img image.Image, from, to colorGamut) image.Image {
	if from == to {
		return img
	}
	b := img.Bounds()
	out := image.NewNRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			v := sampleSDRInGamut(img, x, y, from, to)
			_, _, _, a := img.At(x, y).RGBA()
			out.SetNRGBA(x-b.Min.X, y-b.Min.Y, color.NRGBA{
				R: uint8(clamp01(srgbOetf(v.r))*255.0 + 0.5),
				G: uint8(clamp01(srgbOetf(v.g))*255.0 + 0.5),
				B: uint8(clamp01(srgbOetf(v.b))*255.0 + 0.5),
				A: uint8(a >> 8),
			})
		}
	}
	return out
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
	opt := jpegx.EncoderOptions{
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

func gainmapDecodeValue(v uint8, gamma float32) float32 {
	g := float32(v) / 255.0
	if gamma != 1 {
		g = float32(math.Pow(float64(g), float64(1.0/gamma)))
	}
	return clamp01(g)
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
