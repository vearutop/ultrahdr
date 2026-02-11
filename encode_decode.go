package ultrahdr

import (
	"bytes"
	"errors"
	"image"
	"image/color"
	"image/jpeg"
)

// Encode creates a JPEG/R byte stream from an HDR image and SDR base image.
func Encode(hdr *HDRImage, sdr image.Image, opts *EncodeOptions) ([]byte, *GainMapMetadata, error) {
	if hdr == nil || sdr == nil {
		return nil, nil, errors.New("hdr and sdr must be provided")
	}
	b := sdr.Bounds()
	if b.Dx() != hdr.Width || b.Dy() != hdr.Height {
		return nil, nil, errors.New("hdr and sdr dimensions must match")
	}

	opt := applyEncodeDefaults(opts)
	if opt.GainMapScale < 1 {
		opt.GainMapScale = 1
	}
	if opt.Quality <= 0 {
		opt.Quality = defaultBaseQuality
	}
	if opt.GainMapQuality <= 0 {
		opt.GainMapQuality = defaultGainMapQuality
	}
	if opt.Gamma <= 0 {
		opt.Gamma = defaultGamma
	}
	if opt.HDRWhiteNits <= 0 {
		opt.HDRWhiteNits = defaultHDRWhiteNits
	}

	meta := &GainMapMetadata{Version: jpegrVersion, UseBaseCG: true}
	for i := 0; i < 3; i++ {
		meta.MinContentBoost[i] = 1
		meta.MaxContentBoost[i] = float32(opt.HDRWhiteNits / sdrWhiteNits)
		meta.Gamma[i] = opt.Gamma
		meta.OffsetSDR[i] = 0
		meta.OffsetHDR[i] = 0
	}
	meta.HDRCapacityMin = 1
	if opt.TargetDisplayNits > 0 {
		meta.HDRCapacityMax = float32(opt.TargetDisplayNits / sdrWhiteNits)
	} else {
		meta.HDRCapacityMax = meta.MaxContentBoost[0]
	}

	gainmapImg := generateGainMap(hdr, sdr, meta, opt)

	var baseBuf bytes.Buffer
	if err := jpeg.Encode(&baseBuf, sdr, &jpeg.Options{Quality: opt.Quality}); err != nil {
		return nil, nil, err
	}
	var gmBuf bytes.Buffer
	if err := jpeg.Encode(&gmBuf, gainmapImg, &jpeg.Options{Quality: opt.GainMapQuality}); err != nil {
		return nil, nil, err
	}

	container, err := assembleContainer(baseBuf.Bytes(), gmBuf.Bytes(), meta)
	if err != nil {
		return nil, nil, err
	}
	return container, meta, nil
}

// Decode parses a JPEG/R byte stream into an HDR image and SDR base image.
func Decode(data []byte, opts *DecodeOptions) (*HDRImage, image.Image, *GainMapMetadata, error) {
	if len(data) < 4 {
		return nil, nil, nil, errors.New("input too small")
	}
	ranges, err := scanJPEGs(data)
	if err != nil {
		return nil, nil, nil, err
	}
	if len(ranges) < 2 {
		return nil, nil, nil, errors.New("gainmap image not found")
	}
	primary := data[ranges[0][0]:ranges[0][1]]
	gainmap := data[ranges[1][0]:ranges[1][1]]

	baseImg, err := jpeg.Decode(bytes.NewReader(primary))
	if err != nil {
		return nil, nil, nil, err
	}
	gainmapImg, err := jpeg.Decode(bytes.NewReader(gainmap))
	if err != nil {
		return nil, nil, nil, err
	}

	app1, app2, err := extractAppSegments(gainmap)
	if err != nil {
		return nil, nil, nil, err
	}
	iso := findISO(app2)
	xmp := findXMP(app1)

	var meta *GainMapMetadata
	if iso != nil {
		payload := iso[len(isoNamespace)+1:]
		m, err := decodeGainmapMetadataISO(payload)
		if err != nil {
			return nil, nil, nil, err
		}
		meta = m
	} else if xmp != nil {
		m, err := parseXMP(xmp)
		if err != nil {
			return nil, nil, nil, err
		}
		meta = m
	} else {
		return nil, nil, nil, errors.New("no gainmap metadata found")
	}

	hdr := applyGainMap(baseImg, gainmapImg, meta, opts)
	return hdr, baseImg, meta, nil
}

func applyEncodeDefaults(opts *EncodeOptions) EncodeOptions {
	if opts == nil {
		return EncodeOptions{
			Quality:           defaultBaseQuality,
			GainMapQuality:    defaultGainMapQuality,
			GainMapScale:      defaultGainMapScale,
			UseMultiChannelGM: false,
			Gamma:             defaultGamma,
			HDRWhiteNits:      defaultHDRWhiteNits,
			UseLuminance:      false,
		}
	}
	return *opts
}

func generateGainMap(hdr *HDRImage, sdr image.Image, meta *GainMapMetadata, opt EncodeOptions) image.Image {
	mapW := hdr.Width / opt.GainMapScale
	mapH := hdr.Height / opt.GainMapScale
	if mapW == 0 || mapH == 0 {
		mapW = 1
		mapH = 1
	}
	log2Min := log2f(meta.MinContentBoost[0])
	log2Max := log2f(meta.MaxContentBoost[0])

	if opt.UseMultiChannelGM {
		img := image.NewRGBA(image.Rect(0, 0, mapW, mapH))
		for y := 0; y < mapH; y++ {
			for x := 0; x < mapW; x++ {
				sdrRGB := sampleSDR(sdr, x*opt.GainMapScale, y*opt.GainMapScale)
				hdrRGB := sampleHDR(hdr, x*opt.GainMapScale, y*opt.GainMapScale)
				sdrNits := rgb{r: sdrRGB.r * float32(sdrWhiteNits), g: sdrRGB.g * float32(sdrWhiteNits), b: sdrRGB.b * float32(sdrWhiteNits)}
				hdrNits := rgb{r: hdrRGB.r * float32(opt.HDRWhiteNits), g: hdrRGB.g * float32(opt.HDRWhiteNits), b: hdrRGB.b * float32(opt.HDRWhiteNits)}
				r := encodeGain(sdrNits.r, hdrNits.r, meta, log2Min, log2Max, 0)
				g := encodeGain(sdrNits.g, hdrNits.g, meta, log2Min, log2Max, 1)
				b := encodeGain(sdrNits.b, hdrNits.b, meta, log2Min, log2Max, 2)
				img.SetRGBA(x, y, color.RGBA{R: r, G: g, B: b, A: 255})
			}
		}
		return img
	}

	img := image.NewGray(image.Rect(0, 0, mapW, mapH))
	for y := 0; y < mapH; y++ {
		for x := 0; x < mapW; x++ {
			sdrRGB := sampleSDR(sdr, x*opt.GainMapScale, y*opt.GainMapScale)
			hdrRGB := sampleHDR(hdr, x*opt.GainMapScale, y*opt.GainMapScale)
			var sdrY, hdrY float32
			if opt.UseLuminance {
				sdrY = 0.2126*sdrRGB.r + 0.7152*sdrRGB.g + 0.0722*sdrRGB.b
				hdrY = 0.2126*hdrRGB.r + 0.7152*hdrRGB.g + 0.0722*hdrRGB.b
			} else {
				sdrY = max3(sdrRGB.r, sdrRGB.g, sdrRGB.b)
				hdrY = max3(hdrRGB.r, hdrRGB.g, hdrRGB.b)
			}
			sdrNits := sdrY * float32(sdrWhiteNits)
			hdrNits := hdrY * float32(opt.HDRWhiteNits)
			v := encodeGain(sdrNits, hdrNits, meta, log2Min, log2Max, 0)
			img.SetGray(x, y, color.Gray{Y: v})
		}
	}
	return img
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

func sampleHDR(hdr *HDRImage, x, y int) rgb {
	if x < 0 {
		x = 0
	}
	if y < 0 {
		y = 0
	}
	if x >= hdr.Width {
		x = hdr.Width - 1
	}
	if y >= hdr.Height {
		y = hdr.Height - 1
	}
	idx := y*hdr.Stride + x*3
	return rgb{r: hdr.Pix[idx], g: hdr.Pix[idx+1], b: hdr.Pix[idx+2]}
}

func applyGainMap(base image.Image, gainmap image.Image, meta *GainMapMetadata, opts *DecodeOptions) *HDRImage {
	b := base.Bounds()
	w, h := b.Dx(), b.Dy()
	out := &HDRImage{Width: w, Height: h, Stride: w * 3, Pix: make([]float32, w*h*3)}

	gmBounds := gainmap.Bounds()
	gmW, gmH := gmBounds.Dx(), gmBounds.Dy()
	mapScaleX := float32(w) / float32(gmW)
	mapScaleY := float32(h) / float32(gmH)

	maxBoost := meta.HDRCapacityMax
	if opts != nil && opts.MaxDisplayBoost > 0 {
		maxBoost = opts.MaxDisplayBoost
	}
	weight := float32(1.0)
	if maxBoost < meta.HDRCapacityMax {
		weight = (log2f(maxBoost) - log2f(meta.HDRCapacityMin)) / (log2f(meta.HDRCapacityMax) - log2f(meta.HDRCapacityMin))
		if weight < 0 {
			weight = 0
		}
		if weight > 1 {
			weight = 1
		}
	}

	isGray := isGrayImage(gainmap)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			baseRGB := sampleSDR(base, b.Min.X+x, b.Min.Y+y)

			gx := int(float32(x)/mapScaleX + 0.5)
			gy := int(float32(y)/mapScaleY + 0.5)
			if gx < 0 {
				gx = 0
			}
			if gy < 0 {
				gy = 0
			}
			if gx >= gmW {
				gx = gmW - 1
			}
			if gy >= gmH {
				gy = gmH - 1
			}
			var hdr rgb
			if isGray {
				gyv := grayAt(gainmap, gx, gy)
				gain := float32(gyv) / 255.0
				hdr = applyGainSingle(baseRGB, gain, meta, weight)
			} else {
				gr, gg, gb := rgbAt(gainmap, gx, gy)
				gain := rgb{r: float32(gr) / 255.0, g: float32(gg) / 255.0, b: float32(gb) / 255.0}
				hdr = applyGainRGB(baseRGB, gain, meta, weight)
			}
			idx := y*out.Stride + x*3
			out.Pix[idx] = hdr.r
			out.Pix[idx+1] = hdr.g
			out.Pix[idx+2] = hdr.b
		}
	}
	return out
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

func assembleContainer(primaryJPEG, gainmapJPEG []byte, meta *GainMapMetadata) ([]byte, error) {
	if len(primaryJPEG) < 2 || len(gainmapJPEG) < 2 {
		return nil, errors.New("invalid JPEG data")
	}
	// Build secondary image size first.
	xmpSecondary := generateXmpSecondary(meta)
	xmpSecondaryLen := 2 + len(xmpNamespace) + 1 + len(xmpSecondary)

	isoSecondaryPayload, err := encodeGainmapMetadataISO(meta)
	if err != nil {
		return nil, err
	}
	isoSecondaryLen := 2 + len(isoNamespace) + 1 + len(isoSecondaryPayload)

	secondaryImageSize := len(gainmapJPEG) + 2 + xmpSecondaryLen + 2 + isoSecondaryLen

	var out bytes.Buffer
	writeSOI := func() {
		out.WriteByte(markerStart)
		out.WriteByte(markerSOI)
	}
	writeAPP := func(marker byte, payload []byte) {
		out.WriteByte(markerStart)
		out.WriteByte(marker)
		length := uint16(len(payload) + 2)
		out.WriteByte(byte(length >> 8))
		out.WriteByte(byte(length))
		out.Write(payload)
	}

	writeSOI()

	// XMP primary
	xmpPrimary := generateXmpPrimary(secondaryImageSize, meta)
	payloadPrimary := append(append([]byte{}, []byte(xmpNamespace)...), 0)
	payloadPrimary = append(payloadPrimary, xmpPrimary...)
	writeAPP(markerAPP1, payloadPrimary)

	// ISO 21496-1 version-only
	payloadIsoPrimary := append(append([]byte{}, []byte(isoNamespace)...), 0)
	payloadIsoPrimary = append(payloadIsoPrimary, 0, 0, 0, 0)
	writeAPP(markerAPP2, payloadIsoPrimary)

	// MPF
	mpfLen := 2 + calculateMpfSize()
	primaryImageSize := out.Len() + mpfLen + len(primaryJPEG)
	secondaryOffset := primaryImageSize - out.Len() - 8
	mpf := generateMpf(primaryImageSize, secondaryImageSize, secondaryOffset)
	writeAPP(markerAPP2, mpf)

	// Primary image (skip SOI)
	out.Write(primaryJPEG[2:])

	// Secondary image
	writeSOI()
	payloadSecondary := append(append([]byte{}, []byte(xmpNamespace)...), 0)
	payloadSecondary = append(payloadSecondary, xmpSecondary...)
	writeAPP(markerAPP1, payloadSecondary)

	payloadIsoSecondary := append(append([]byte{}, []byte(isoNamespace)...), 0)
	payloadIsoSecondary = append(payloadIsoSecondary, isoSecondaryPayload...)
	writeAPP(markerAPP2, payloadIsoSecondary)

	out.Write(gainmapJPEG[2:])

	return out.Bytes(), nil
}
