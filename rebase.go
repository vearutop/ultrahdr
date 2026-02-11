package ultrahdr

import (
	"bytes"
	"errors"
	"image"
	"image/color"
	"math"
	"os"
	"path/filepath"
)

// RebaseOptions controls gainmap rebase behavior.
type RebaseOptions struct {
	BaseQuality    int
	GainmapQuality int
}

// RebaseResult contains the rebased container and component JPEGs.
type RebaseResult struct {
	Container []byte
	Primary   []byte
	Gainmap   []byte
}

// RebaseUltraHDR replaces the primary SDR image while adjusting the gainmap
// to preserve the original HDR reconstruction as closely as possible.
func RebaseUltraHDR(data []byte, newSDR image.Image, opt *RebaseOptions) (*RebaseResult, error) {
	if newSDR == nil {
		return nil, errors.New("new SDR image is nil")
	}
	split, err := Split(data)
	if err != nil {
		return nil, err
	}
	if split.Meta == nil {
		return nil, errors.New("gainmap metadata missing")
	}
	oldSDR, _, err := image.Decode(bytes.NewReader(split.PrimaryJPEG))
	if err != nil {
		return nil, err
	}
	gainmapImg, _, err := image.Decode(bytes.NewReader(split.GainmapJPEG))
	if err != nil {
		return nil, err
	}
	if oldSDR.Bounds().Dx() != newSDR.Bounds().Dx() || oldSDR.Bounds().Dy() != newSDR.Bounds().Dy() {
		return nil, errors.New("new SDR dimensions must match original")
	}

	gainmapOut, err := rebaseGainmap(oldSDR, newSDR, gainmapImg, split.Meta)
	if err != nil {
		return nil, err
	}

	gainQ := defaultGainMapQuality
	baseQ := defaultPrimaryQuality
	if opt != nil {
		if opt.GainmapQuality > 0 {
			gainQ = opt.GainmapQuality
		}
		if opt.BaseQuality > 0 {
			baseQ = opt.BaseQuality
		}
	}
	gainmapJpeg, err := encodeWithQuality(gainmapOut, gainQ)
	if err != nil {
		return nil, err
	}

	primaryOut, err := encodeWithQuality(newSDR, baseQ)
	if err != nil {
		return nil, err
	}

	exif, icc, err := extractExifAndIcc(primaryOut)
	if err != nil {
		return nil, err
	}
	if len(exif) == 0 && len(icc) == 0 {
		exif, icc, err = extractExifAndIcc(split.PrimaryJPEG)
		if err != nil {
			return nil, err
		}
	}
	container, err := assembleContainerVipsLike(primaryOut, gainmapJpeg, exif, icc, split.Segs.SecondaryXMP, split.Segs.SecondaryISO)
	if err != nil {
		return nil, err
	}
	return &RebaseResult{
		Container: container,
		Primary:   primaryOut,
		Gainmap:   gainmapJpeg,
	}, nil
}

// RebaseUltraHDRFile reads an UltraHDR JPEG, rebases it on newSDRPath, and writes the output.
func RebaseUltraHDRFile(inPath, newSDRPath, outPath string, opt *RebaseOptions, primaryOut, gainmapOut string) error {
	data, err := os.ReadFile(filepath.Clean(inPath))
	if err != nil {
		return err
	}
	newSDRFile, err := os.Open(filepath.Clean(newSDRPath))
	if err != nil {
		return err
	}
	defer newSDRFile.Close()
	newSDR, _, err := image.Decode(newSDRFile)
	if err != nil {
		return err
	}
	res, err := RebaseUltraHDR(data, newSDR, opt)
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Clean(outPath), res.Container, 0o644); err != nil {
		return err
	}
	if primaryOut != "" {
		if err := os.WriteFile(filepath.Clean(primaryOut), res.Primary, 0o644); err != nil {
			return err
		}
	}
	if gainmapOut != "" {
		if err := os.WriteFile(filepath.Clean(gainmapOut), res.Gainmap, 0o644); err != nil {
			return err
		}
	}
	return nil
}

func rebaseGainmap(oldSDR, newSDR, gainmap image.Image, meta *GainMapMetadata) (image.Image, error) {
	if meta == nil {
		return nil, errors.New("gainmap metadata missing")
	}
	b := newSDR.Bounds()
	w, h := b.Dx(), b.Dy()
	gmBounds := gainmap.Bounds()
	gmW, gmH := gmBounds.Dx(), gmBounds.Dy()
	mapScaleX := float32(w) / float32(gmW)
	mapScaleY := float32(h) / float32(gmH)

	isGray := isGrayImage(gainmap)
	if isGray {
		out := image.NewGray(image.Rect(0, 0, w, h))
		for y := 0; y < h; y++ {
			for x := 0; x < w; x++ {
				oldRGB := sampleSDR(oldSDR, b.Min.X+x, b.Min.Y+y)
				newRGB := sampleSDR(newSDR, b.Min.X+x, b.Min.Y+y)
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
				gv := gainmapDecodeValue(grayAt(gainmap, gx, gy), meta.Gamma[0])
				logBoost := log2f(meta.MinContentBoost[0])*(1.0-gv) + log2f(meta.MaxContentBoost[0])*gv
				gainFactor := exp2f(logBoost)
				hdr := rgb{
					r: (oldRGB.r+meta.OffsetSDR[0])*gainFactor - meta.OffsetHDR[0],
					g: (oldRGB.g+meta.OffsetSDR[0])*gainFactor - meta.OffsetHDR[0],
					b: (oldRGB.b+meta.OffsetSDR[0])*gainFactor - meta.OffsetHDR[0],
				}
				hdrY := max3(hdr.r, hdr.g, hdr.b)
				newY := max3(newRGB.r, newRGB.g, newRGB.b)
				denom := newY + meta.OffsetSDR[0]
				if denom <= 0 {
					denom = 1e-6
				}
				newGain := (hdrY + meta.OffsetHDR[0]) / denom
				newGV := gainFromFactor(newGain, meta.MinContentBoost[0], meta.MaxContentBoost[0], meta.Gamma[0])
				out.SetGray(x, y, color.Gray{Y: newGV})
			}
		}
		return out, nil
	}

	out := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			oldRGB := sampleSDR(oldSDR, b.Min.X+x, b.Min.Y+y)
			newRGB := sampleSDR(newSDR, b.Min.X+x, b.Min.Y+y)
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
			gr, gg, gb := rgbAt(gainmap, gx, gy)
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
			hdr := rgb{
				r: (oldRGB.r+meta.OffsetSDR[0])*gainFactorR - meta.OffsetHDR[0],
				g: (oldRGB.g+meta.OffsetSDR[1])*gainFactorG - meta.OffsetHDR[1],
				b: (oldRGB.b+meta.OffsetSDR[2])*gainFactorB - meta.OffsetHDR[2],
			}
			denomR := newRGB.r + meta.OffsetSDR[0]
			denomG := newRGB.g + meta.OffsetSDR[1]
			denomB := newRGB.b + meta.OffsetSDR[2]
			if denomR <= 0 {
				denomR = 1e-6
			}
			if denomG <= 0 {
				denomG = 1e-6
			}
			if denomB <= 0 {
				denomB = 1e-6
			}
			newGainR := (hdr.r + meta.OffsetHDR[0]) / denomR
			newGainG := (hdr.g + meta.OffsetHDR[1]) / denomG
			newGainB := (hdr.b + meta.OffsetHDR[2]) / denomB
			out.SetRGBA(x, y, color.RGBA{
				R: gainFromFactor(newGainR, meta.MinContentBoost[0], meta.MaxContentBoost[0], meta.Gamma[0]),
				G: gainFromFactor(newGainG, meta.MinContentBoost[1], meta.MaxContentBoost[1], meta.Gamma[1]),
				B: gainFromFactor(newGainB, meta.MinContentBoost[2], meta.MaxContentBoost[2], meta.Gamma[2]),
				A: 0xFF,
			})
		}
	}
	return out, nil
}

func gainFromFactor(gainFactor, minBoost, maxBoost, gamma float32) uint8 {
	if gainFactor < minBoost {
		gainFactor = minBoost
	}
	if gainFactor > maxBoost {
		gainFactor = maxBoost
	}
	logBoost := log2f(gainFactor)
	logMin := log2f(minBoost)
	logMax := log2f(maxBoost)
	g := float32(0)
	if logMax != logMin {
		g = (logBoost - logMin) / (logMax - logMin)
	}
	g = clamp01(g)
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
