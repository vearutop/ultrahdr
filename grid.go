package ultrahdr

import (
	"bytes"
	"errors"
	"image"
	"image/color"
	"image/draw"
	"io"
	"math"
)

// GridOptions configures grid rendering for SDR inputs.
type GridOptions struct {
	Quality       int           // JPEG quality for the output (0 uses default).
	Interpolation Interpolation // Resize interpolation mode.
	Background    color.Color   // Background fill color (nil uses black).
}

// Grid builds a sprite grid from SDR images. Inputs are resized to fit each cell
// preserving aspect ratio and padded with black. Output is encoded as sRGB JPEG.
func Grid(readers []io.Reader, cols int, cellW, cellH int, opts *GridOptions) (*Result, error) {
	if len(readers) == 0 {
		return nil, errors.New("no input images")
	}
	if cols <= 0 {
		return nil, errors.New("invalid columns")
	}
	if cellW <= 0 || cellH <= 0 {
		return nil, errors.New("invalid cell size")
	}

	quality := defaultPrimaryQuality
	interp := InterpolationLanczos2
	if opts != nil {
		if opts.Quality > 0 {
			quality = opts.Quality
		}
		if opts.Interpolation != 0 {
			interp = opts.Interpolation
		}
	}

	rows := int(math.Ceil(float64(len(readers)) / float64(cols)))
	gridW := cols * cellW
	gridH := rows * cellH
	grid := image.NewNRGBA(image.Rect(0, 0, gridW, gridH))
	bg := color.NRGBA{A: 0xFF}
	if opts != nil && opts.Background != nil {
		bg = color.NRGBAModel.Convert(opts.Background).(color.NRGBA)
	}
	draw.Draw(grid, grid.Bounds(), &image.Uniform{C: bg}, image.Point{}, draw.Src)

	gridHDR := &hdrImage{W: gridW, H: gridH, Pix: make([]float32, gridW*gridH*3)}
	fillHDRBackground(gridHDR, bg)
	hasHDR := false
	sdrProfile := colorProfile{gamut: colorGamutSRGB, transfer: colorTransferSRGB}

	for idx, r := range readers {
		if r == nil {
			return nil, errors.New("nil reader")
		}
		data, err := io.ReadAll(r)
		if err != nil {
			return nil, err
		}
		input, err := decodeGridInput(data)
		if err != nil {
			return nil, err
		}
		if input.sdr == nil {
			return nil, errors.New("missing SDR input")
		}

		if input.profile != sdrProfile {
			input.sdr = convertImageProfile(input.sdr, input.profile, sdrProfile)
		}

		resized, w, h := resizeToFit(input.sdr, cellW, cellH, interp)
		col := idx % cols
		row := idx / cols
		x0 := col*cellW + (cellW-w)/2
		y0 := row*cellH + (cellH-h)/2
		dstRect := image.Rect(x0, y0, x0+w, y0+h)
		draw.Draw(grid, dstRect, resized, resized.Bounds().Min, draw.Src)

		if input.gainmap != nil && input.meta != nil {
			hasHDR = true
			gainmap := input.gainmap
			if input.gainmap.Bounds().Dx() != w || input.gainmap.Bounds().Dy() != h {
				gainmap = resizeImageInterpolated(input.gainmap, w, h, interp)
			}
			writeHDRTile(gridHDR, resized, gainmap, input.meta, x0, y0)
		} else {
			writeHDRTile(gridHDR, resized, nil, nil, x0, y0)
		}
	}

	out, err := encodeWithQuality(grid, quality)
	if err != nil {
		return nil, err
	}
	if !hasHDR {
		return &Result{Container: out, Primary: out}, nil
	}

	gainmapImg, meta, err := generateGainmapFromHDR(grid, sdrProfile, gridHDR, nil)
	if err != nil {
		return nil, err
	}
	gainmapJPEG, err := encodeWithQuality(gainmapImg, defaultGainMapQuality)
	if err != nil {
		return nil, err
	}
	secondaryISO, err := buildIsoPayload(meta)
	if err != nil {
		return nil, err
	}
	container, err := assembleContainerVipsLike(out, gainmapJPEG, nil, nil, nil, secondaryISO)
	if err != nil {
		return nil, err
	}
	return &Result{
		Container: container,
		Primary:   out,
		Gainmap:   gainmapJPEG,
		Meta:      meta,
	}, nil
}

func resizeToFit(img image.Image, cellW, cellH int, interp Interpolation) (image.Image, int, int) {
	b := img.Bounds()
	sw := b.Dx()
	sh := b.Dy()
	if sw <= 0 || sh <= 0 {
		return image.NewNRGBA(image.Rect(0, 0, 1, 1)), 1, 1
	}

	scaleX := float64(cellW) / float64(sw)
	scaleY := float64(cellH) / float64(sh)
	scale := scaleX
	if scaleY < scaleX {
		scale = scaleY
	}
	if scale <= 0 {
		scale = 1
	}

	newW := int(math.Round(float64(sw) * scale))
	newH := int(math.Round(float64(sh) * scale))
	if newW < 1 {
		newW = 1
	}
	if newH < 1 {
		newH = 1
	}
	if newW == sw && newH == sh {
		return img, newW, newH
	}
	resized := resizeImageInterpolated(img, newW, newH, interp)
	return resized, newW, newH
}

type gridInput struct {
	sdr     image.Image
	gainmap image.Image
	meta    *GainMapMetadata
	profile colorProfile
}

func decodeGridInput(data []byte) (*gridInput, error) {
	if len(data) == 0 {
		return nil, errors.New("empty input")
	}
	srcProfile := colorProfile{gamut: colorGamutSRGB, transfer: colorTransferSRGB}
	_, icc, err := extractExifAndIcc(data)
	if err == nil {
		srcProfile = detectColorProfileFromICCProfile(collectICCProfile(icc))
	}

	split, err := Split(bytes.NewReader(data))
	if err != nil || split == nil || split.Meta == nil {
		img, _, err := image.Decode(bytes.NewReader(data))
		if err != nil {
			return nil, err
		}
		return &gridInput{sdr: img, profile: srcProfile}, nil
	}

	primaryImg, _, err := image.Decode(bytes.NewReader(split.Primary))
	if err != nil {
		return nil, err
	}
	gainmapImg, _, err := image.Decode(bytes.NewReader(split.Gainmap))
	if err != nil {
		return nil, err
	}
	return &gridInput{
		sdr:     primaryImg,
		gainmap: gainmapImg,
		meta:    split.Meta,
		profile: srcProfile,
	}, nil
}

func writeHDRTile(dst *hdrImage, sdr image.Image, gainmap image.Image, meta *GainMapMetadata, x0, y0 int) {
	if dst == nil || sdr == nil {
		return
	}
	b := sdr.Bounds()
	w := b.Dx()
	h := b.Dy()
	isGray := false
	if gainmap != nil {
		isGray = isGrayImage(gainmap)
	}
	srcProfile := colorProfile{gamut: colorGamutSRGB, transfer: colorTransferSRGB}

	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			sdrRGB := sampleSDRInProfile(sdr, b.Min.X+x, b.Min.Y+y, srcProfile, colorGamutSRGB)
			hdrRGB := sdrRGB
			if gainmap != nil && meta != nil {
				hdrRGB = applyGainmapToSDR(sdrRGB, gainmap, meta, x, y, isGray)
			}
			dst.set(x0+x, y0+y, hdrRGB)
		}
	}
}

func (h *hdrImage) set(x, y int, v rgb) {
	if h == nil || x < 0 || y < 0 || x >= h.W || y >= h.H {
		return
	}
	i := (y*h.W + x) * 3
	h.Pix[i] = v.r
	h.Pix[i+1] = v.g
	h.Pix[i+2] = v.b
}

func applyGainmapToSDR(sdr rgb, gainmap image.Image, meta *GainMapMetadata, x, y int, isGray bool) rgb {
	if gainmap == nil || meta == nil {
		return sdr
	}
	if isGray {
		gv := gainmapDecodeValue(grayAt(gainmap, x, y), meta.Gamma[0])
		logBoost := log2f(meta.MinContentBoost[0])*(1.0-gv) + log2f(meta.MaxContentBoost[0])*gv
		gainFactor := exp2f(logBoost)
		return rgb{
			r: (sdr.r+meta.OffsetSDR[0])*gainFactor - meta.OffsetHDR[0],
			g: (sdr.g+meta.OffsetSDR[0])*gainFactor - meta.OffsetHDR[0],
			b: (sdr.b+meta.OffsetSDR[0])*gainFactor - meta.OffsetHDR[0],
		}
	}

	gr, gg, gb := rgbAt(gainmap, x, y)
	gain := rgb{
		r: gainmapDecodeValue(gr, meta.Gamma[0]),
		g: gainmapDecodeValue(gg, meta.Gamma[1]),
		b: gainmapDecodeValue(gb, meta.Gamma[2]),
	}
	logBoostR := log2f(meta.MinContentBoost[0])*(1.0-gain.r) + log2f(meta.MaxContentBoost[0])*gain.r
	logBoostG := log2f(meta.MinContentBoost[1])*(1.0-gain.g) + log2f(meta.MaxContentBoost[1])*gain.g
	logBoostB := log2f(meta.MinContentBoost[2])*(1.0-gain.b) + log2f(meta.MaxContentBoost[2])*gain.b
	gainFactorR := exp2f(logBoostR)
	gainFactorG := exp2f(logBoostG)
	gainFactorB := exp2f(logBoostB)
	return rgb{
		r: (sdr.r+meta.OffsetSDR[0])*gainFactorR - meta.OffsetHDR[0],
		g: (sdr.g+meta.OffsetSDR[1])*gainFactorG - meta.OffsetHDR[1],
		b: (sdr.b+meta.OffsetSDR[2])*gainFactorB - meta.OffsetHDR[2],
	}
}

func fillHDRBackground(dst *hdrImage, bg color.NRGBA) {
	if dst == nil {
		return
	}
	r := invOETF(float32(bg.R)/255.0, colorTransferSRGB)
	g := invOETF(float32(bg.G)/255.0, colorTransferSRGB)
	b := invOETF(float32(bg.B)/255.0, colorTransferSRGB)
	for i := 0; i < len(dst.Pix); i += 3 {
		dst.Pix[i] = r
		dst.Pix[i+1] = g
		dst.Pix[i+2] = b
	}
}
