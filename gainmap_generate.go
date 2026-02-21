package ultrahdr

import (
	"errors"
	"fmt"
	"image"
	"image/color"
	"math"
)

const (
	kSdrWhiteNits = 203.0
	kSdrOffset    = 1e-7
	kHdrOffset    = 1e-7
)

func generateGainmapFromHDR(sdr image.Image, sdrProfile colorProfile, hdr *hdrImage, opt *RebaseOptions) (image.Image, *GainMapMetadata, error) {
	if sdr == nil || hdr == nil {
		return nil, nil, errors.New("missing SDR or HDR input")
	}
	b := sdr.Bounds()
	if b.Dx() != hdr.W || b.Dy() != hdr.H {
		return nil, nil, fmt.Errorf("SDR and HDR dimensions must match: %dx%d vs %dx%d", b.Dx(), b.Dy(), hdr.W, hdr.H)
	}
	scale := 1
	gamma := float32(1.0)
	useMulti := false
	if opt != nil {
		if opt.GainmapScale > 0 {
			scale = opt.GainmapScale
		}
		if opt.GainmapGamma > 0 {
			gamma = opt.GainmapGamma
		}
		if opt.UseMultiChannel {
			useMulti = true
		}
	}
	if scale <= 0 {
		scale = 1
	}
	mapW := b.Dx() / scale
	mapH := b.Dy() / scale
	if mapW <= 0 || mapH <= 0 {
		return nil, nil, errors.New("gainmap scale too large")
	}

	channels := 1
	if useMulti {
		channels = 3
	}
	gainmapData := make([]float32, mapW*mapH*channels)
	gainMin := make([]float32, channels)
	gainMax := make([]float32, channels)
	for i := 0; i < channels; i++ {
		gainMin[i] = float32(math.MaxFloat32)
		gainMax[i] = -float32(math.MaxFloat32)
	}

	for y := 0; y < mapH; y++ {
		srcY := b.Min.Y + y*scale
		for x := 0; x < mapW; x++ {
			srcX := b.Min.X + x*scale
			sdrRGB := sampleSDRInProfile(sdr, srcX, srcY, sdrProfile, sdrProfile.gamut)
			hdrRGB := hdr.at(srcX-b.Min.X, srcY-b.Min.Y)
			hdrRGB = clampRGB(hdrRGB)
			sdrRGB = clampRGB(sdrRGB)

			if useMulti {
				sdrR := float32(kSdrWhiteNits) * sdrRGB.r
				sdrG := float32(kSdrWhiteNits) * sdrRGB.g
				sdrB := float32(kSdrWhiteNits) * sdrRGB.b
				hdrR := float32(kSdrWhiteNits) * hdrRGB.r
				hdrG := float32(kSdrWhiteNits) * hdrRGB.g
				hdrB := float32(kSdrWhiteNits) * hdrRGB.b
				g0 := computeGain(sdrR, hdrR)
				g1 := computeGain(sdrG, hdrG)
				g2 := computeGain(sdrB, hdrB)
				idx := (y*mapW + x) * 3
				gainmapData[idx] = g0
				gainmapData[idx+1] = g1
				gainmapData[idx+2] = g2
				updateMinMax(gainMin, gainMax, g0, g1, g2)
			} else {
				sdrY := float32(kSdrWhiteNits) * max3(sdrRGB.r, sdrRGB.g, sdrRGB.b)
				hdrY := float32(kSdrWhiteNits) * max3(hdrRGB.r, hdrRGB.g, hdrRGB.b)
				g := computeGain(sdrY, hdrY)
				idx := y*mapW + x
				gainmapData[idx] = g
				if g < gainMin[0] {
					gainMin[0] = g
				}
				if g > gainMax[0] {
					gainMax[0] = g
				}
			}
		}
	}

	for i := 0; i < channels; i++ {
		gainMin[i] = clampGainLog2(gainMin[i])
		gainMax[i] = clampGainLog2(gainMax[i])
		if gainMax[i]-gainMin[i] < 1e-6 {
			gainMax[i] = gainMin[i] + 0.1
		}
	}

	var gainmap image.Image
	if useMulti {
		out := image.NewRGBA(image.Rect(0, 0, mapW, mapH))
		for y := 0; y < mapH; y++ {
			for x := 0; x < mapW; x++ {
				idx := (y*mapW + x) * 3
				r := affineMapGain(gainmapData[idx], gainMin[0], gainMax[0], gamma)
				g := affineMapGain(gainmapData[idx+1], gainMin[1], gainMax[1], gamma)
				bc := affineMapGain(gainmapData[idx+2], gainMin[2], gainMax[2], gamma)
				out.SetRGBA(x, y, color.RGBA{R: r, G: g, B: bc, A: 0xFF})
			}
		}
		gainmap = out
	} else {
		out := image.NewGray(image.Rect(0, 0, mapW, mapH))
		for y := 0; y < mapH; y++ {
			for x := 0; x < mapW; x++ {
				idx := y*mapW + x
				v := affineMapGain(gainmapData[idx], gainMin[0], gainMax[0], gamma)
				out.SetGray(x, y, color.Gray{Y: v})
			}
		}
		gainmap = out
	}

	meta := &GainMapMetadata{
		Version:        jpegrVersion,
		UseBaseCG:      true,
		HDRCapacityMin: 1.0,
	}
	if useMulti {
		for i := 0; i < 3; i++ {
			meta.MinContentBoost[i] = exp2f(gainMin[i])
			meta.MaxContentBoost[i] = exp2f(gainMax[i])
			meta.Gamma[i] = gamma
			meta.OffsetSDR[i] = kSdrOffset
			meta.OffsetHDR[i] = kHdrOffset
		}
		meta.HDRCapacityMax = meta.MaxContentBoost[0]
	} else {
		minBoost := exp2f(gainMin[0])
		maxBoost := exp2f(gainMax[0])
		for i := 0; i < 3; i++ {
			meta.MinContentBoost[i] = minBoost
			meta.MaxContentBoost[i] = maxBoost
			meta.Gamma[i] = gamma
			meta.OffsetSDR[i] = kSdrOffset
			meta.OffsetHDR[i] = kHdrOffset
		}
		meta.HDRCapacityMax = maxBoost
	}
	return gainmap, meta, nil
}

func clampRGB(v rgb) rgb {
	if v.r < 0 {
		v.r = 0
	}
	if v.g < 0 {
		v.g = 0
	}
	if v.b < 0 {
		v.b = 0
	}
	return v
}

func computeGain(sdr, hdr float32) float32 {
	gain := log2f((hdr + kHdrOffset) / (sdr + kSdrOffset))
	if sdr < 2.0/255.0 {
		if gain > 2.3 {
			gain = 2.3
		}
	}
	return gain
}

func clampGainLog2(v float32) float32 {
	if v < -14.3 {
		return -14.3
	}
	if v > 15.6 {
		return 15.6
	}
	return v
}

func affineMapGain(gainlog2, minlog2, maxlog2, gamma float32) uint8 {
	denom := maxlog2 - minlog2
	if denom == 0 {
		denom = 1
	}
	mapped := (gainlog2 - minlog2) / denom
	if mapped < 0 {
		mapped = 0
	}
	if mapped > 1 {
		mapped = 1
	}
	if gamma != 1 {
		mapped = float32(math.Pow(float64(mapped), float64(gamma)))
	}
	val := mapped * 255
	if val < 0 {
		val = 0
	}
	if val > 255 {
		val = 255
	}
	return uint8(val + 0.5)
}

func updateMinMax(minv, maxv []float32, r, g, b float32) {
	if r < minv[0] {
		minv[0] = r
	}
	if r > maxv[0] {
		maxv[0] = r
	}
	if len(minv) < 3 {
		return
	}
	if g < minv[1] {
		minv[1] = g
	}
	if g > maxv[1] {
		maxv[1] = g
	}
	if b < minv[2] {
		minv[2] = b
	}
	if b > maxv[2] {
		maxv[2] = b
	}
}
