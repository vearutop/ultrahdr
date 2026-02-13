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

// ResizeSpec describes one output variant for ResizeJPEGBatch.
type ResizeSpec struct {
	Width         uint
	Height        uint
	Quality       int
	Interpolation Interpolation
	KeepMeta      bool
	ReceiveResult func(data []byte, err error)
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
// When keepMeta is false and input is a wide-gamut profile, output pixels are converted to sRGB.
func ResizeJPEG(data []byte, width, height uint, quality int, interp Interpolation, keepMeta bool) ([]byte, error) {
	var (
		res []byte
		err error
	)
	specs := []ResizeSpec{{
		Width:         width,
		Height:        height,
		Quality:       quality,
		Interpolation: interp,
		KeepMeta:      keepMeta,
		ReceiveResult: func(d []byte, e error) {
			res = d
			err = e
		},
	}}
	err = ResizeJPEGBatch(data, specs)
	if err != nil {
		return nil, err
	}
	return res, err
}

// ResizeJPEGBatch resizes one JPEG into multiple outputs with a single source decode.
// For each spec: when KeepMeta is true EXIF/ICC are preserved; otherwise output is metadata-free.
// Metadata-free outputs are converted to sRGB when source profile is recognized as wide gamut.
func ResizeJPEGBatch(data []byte, specs []ResizeSpec) error {
	if len(specs) == 0 {
		return errors.New("no resize specs provided")
	}

	for _, s := range specs {
		if s.Width == 0 || s.Height == 0 {
			return errors.New("invalid target dimensions")
		}
	}

	srcProfile := colorProfile{gamut: colorGamutSRGB, transfer: colorTransferSRGB}
	exif, icc, err := extractExifAndIcc(data)
	if err == nil {
		srcProfile = detectColorProfileFromICCProfile(collectICCProfile(icc))
	}

	keepMetaSegs := make([]appSegment, 0, 1+len(icc))
	if exif != nil {
		keepMetaSegs = append(keepMetaSegs, appSegment{marker: markerAPP1, payload: exif})
	}
	for _, seg := range icc {
		keepMetaSegs = append(keepMetaSegs, appSegment{marker: markerAPP2, payload: seg})
	}

	srcImg, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return err
	}

	type resizedKey struct {
		w      int
		h      int
		interp Interpolation
	}
	type convertedKey struct {
		resizedKey

		profile colorProfile
	}

	resizedCache := map[resizedKey]image.Image{}
	convertedCache := map[convertedKey]image.Image{}

	for _, spec := range specs {
		rk := resizedKey{w: int(spec.Width), h: int(spec.Height), interp: spec.Interpolation}
		resized, ok := resizedCache[rk]
		if !ok {
			resized = resizeImageInterpolated(srcImg, rk.w, rk.h, rk.interp)
			resizedCache[rk] = resized
		}

		dstProfile := srcProfile
		var segs []appSegment
		if spec.KeepMeta {
			segs = keepMetaSegs
		} else {
			dstProfile = colorProfile{gamut: colorGamutSRGB, transfer: colorTransferSRGB}
		}

		ck := convertedKey{resizedKey: rk, profile: dstProfile}
		converted, ok := convertedCache[ck]
		if !ok {
			converted = resized
			if dstProfile != srcProfile {
				converted = convertImageProfile(converted, srcProfile, dstProfile)
			}
			convertedCache[ck] = converted
		}

		out, err := encodeWithQuality(converted, spec.Quality)
		if err != nil {
			if spec.ReceiveResult != nil {
				spec.ReceiveResult(nil, err)
			}
		}
		if len(segs) > 0 {
			out, err = insertAppSegments(out, segs)
			if err != nil {
				if spec.ReceiveResult != nil {
					spec.ReceiveResult(nil, err)
				}
			}
		}

		if spec.ReceiveResult != nil {
			spec.ReceiveResult(out, err)
		}
	}

	return nil
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
	srgbProfile := colorProfile{gamut: colorGamutSRGB, transfer: colorTransferSRGB}
	return resizeJPEGWithProfile(jpegData, w, h, segs, quality, interp, srgbProfile, srgbProfile)
}

func resizeJPEGWithProfile(jpegData []byte, w, h uint, segs []appSegment, quality int, interp Interpolation, srcProfile, dstProfile colorProfile) ([]byte, error) {
	img, _, err := image.Decode(bytes.NewReader(jpegData))
	if err != nil {
		return nil, err
	}
	outImg := resizeImageInterpolated(img, int(w), int(h), interp)
	if srcProfile != dstProfile {
		outImg = convertImageProfile(outImg, srcProfile, dstProfile)
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

func convertImageProfile(img image.Image, from, to colorProfile) image.Image {
	if from == to {
		return img
	}
	b := img.Bounds()
	out := image.NewNRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			v := sampleSDRInProfile(img, x, y, from, to.gamut)
			_, _, _, a := img.At(x, y).RGBA()
			out.SetNRGBA(x-b.Min.X, y-b.Min.Y, color.NRGBA{
				R: uint8(clamp01(oETF(v.r, to.transfer))*255.0 + 0.5),
				G: uint8(clamp01(oETF(v.g, to.transfer))*255.0 + 0.5),
				B: uint8(clamp01(oETF(v.b, to.transfer))*255.0 + 0.5),
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
