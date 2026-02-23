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
	BaseQuality     int     // JPEG quality for the primary SDR output (0 uses default).
	GainmapQuality  int     // JPEG quality for the gainmap output (0 uses default).
	GainmapScale    int     // Downscale factor for gainmap generation (higher is smaller/faster).
	GainmapGamma    float32 // Gamma to apply to gainmap encoding (0 uses default).
	UseMultiChannel bool    // Encode gainmap as RGB instead of single-channel.
	HDRCapacityMax  float32 // Clamp maximum HDR capacity when generating gainmaps.
	ICCProfile      []byte  // ICC profile bytes for new SDR when not embedded in input.
	PrimaryOut      string  // Optional output path for the rebased primary JPEG.
	GainmapOut      string  // Optional output path for the rebased gainmap JPEG.
}

// RebaseOption configures rebase behavior.
type RebaseOption func(*RebaseOptions)

// WithBaseQuality sets the JPEG quality for the primary SDR output.
func WithBaseQuality(quality int) RebaseOption {
	return func(opt *RebaseOptions) {
		opt.BaseQuality = quality
	}
}

// WithGainmapQuality sets the JPEG quality for the gainmap output.
func WithGainmapQuality(quality int) RebaseOption {
	return func(opt *RebaseOptions) {
		opt.GainmapQuality = quality
	}
}

// WithGainmapScale sets the downscale factor for gainmap generation.
func WithGainmapScale(scale int) RebaseOption {
	return func(opt *RebaseOptions) {
		opt.GainmapScale = scale
	}
}

// WithGainmapGamma sets the gamma to apply to gainmap encoding.
func WithGainmapGamma(gamma float32) RebaseOption {
	return func(opt *RebaseOptions) {
		opt.GainmapGamma = gamma
	}
}

// WithMultiChannelGainmap toggles RGB gainmap encoding.
func WithMultiChannelGainmap(enabled bool) RebaseOption {
	return func(opt *RebaseOptions) {
		opt.UseMultiChannel = enabled
	}
}

// WithHDRCapacityMax clamps maximum HDR capacity when generating gainmaps.
func WithHDRCapacityMax(limit float32) RebaseOption {
	return func(opt *RebaseOptions) {
		opt.HDRCapacityMax = limit
	}
}

// WithICCProfile sets the ICC profile bytes for the new SDR image.
func WithICCProfile(profile []byte) RebaseOption {
	return func(opt *RebaseOptions) {
		opt.ICCProfile = profile
	}
}

// WithPrimaryOut sets an optional output path for the rebased primary JPEG.
func WithPrimaryOut(path string) RebaseOption {
	return func(opt *RebaseOptions) {
		opt.PrimaryOut = path
	}
}

// WithGainmapOut sets an optional output path for the rebased gainmap JPEG.
func WithGainmapOut(path string) RebaseOption {
	return func(opt *RebaseOptions) {
		opt.GainmapOut = path
	}
}

func applyRebaseOptions(opts []RebaseOption) *RebaseOptions {
	if len(opts) == 0 {
		return nil
	}
	cfg := &RebaseOptions{}
	for _, opt := range opts {
		if opt != nil {
			opt(cfg)
		}
	}
	return cfg
}

// RebaseResult contains the rebased container and component JPEGs.
type RebaseResult struct {
	Container []byte
	Primary   []byte
	Gainmap   []byte
	Meta      *GainMapMetadata
}

// Rebase replaces the primary SDR image while adjusting the gainmap
// to preserve the original HDR reconstruction as closely as possible.
func Rebase(data []byte, newSDR image.Image, opts ...RebaseOption) (*RebaseResult, error) {
	opt := applyRebaseOptions(opts)
	return rebaseWithOptions(data, newSDR, opt)
}

func rebaseWithOptions(data []byte, newSDR image.Image, opt *RebaseOptions) (*RebaseResult, error) {
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

	_, oldICCSegs, err := extractExifAndIcc(split.PrimaryJPEG)
	if err != nil {
		return nil, err
	}
	oldICCProfile := collectICCProfile(oldICCSegs)
	oldProfile := detectColorProfileFromICCProfile(oldICCProfile)
	workGamut := oldProfile.gamut
	newProfile := oldProfile
	if opt != nil && len(opt.ICCProfile) > 0 {
		newProfile = detectColorProfileFromICCProfile(opt.ICCProfile)
	}

	gainmapOut, err := rebaseGainmap(oldSDR, newSDR, gainmapImg, split.Meta, oldProfile, newProfile, workGamut)
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
	secondaryISO := split.Segs.SecondaryISO
	if len(secondaryISO) == 0 && split.Meta != nil {
		secondaryISO, err = buildIsoPayload(split.Meta)
		if err != nil {
			return nil, err
		}
	}
	container, err := assembleContainerVipsLike(primaryOut, gainmapJpeg, exif, icc, split.Segs.SecondaryXMP, secondaryISO)
	if err != nil {
		return nil, err
	}
	return &RebaseResult{
		Container: container,
		Primary:   primaryOut,
		Gainmap:   gainmapJpeg,
		Meta:      split.Meta,
	}, nil
}

func rebaseUltraHDRFromHDR(newSDR image.Image, hdr *hdrImage, opt *RebaseOptions) (*RebaseResult, error) {
	if newSDR == nil || hdr == nil {
		return nil, errors.New("missing SDR or HDR input")
	}
	var iccProfile []byte
	if opt != nil {
		iccProfile = opt.ICCProfile
	}
	newProfile := detectColorProfileFromICCProfile(iccProfile)
	gainmapOut, meta, err := generateGainmapFromHDR(newSDR, newProfile, hdr, opt)
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
	return &RebaseResult{
		Primary: primaryOut,
		Gainmap: gainmapJpeg,
		Meta:    meta,
	}, nil
}

// RebaseFile reads an UltraHDR JPEG, rebases it on newSDRPath, and writes the output.
func RebaseFile(inPath, newSDRPath, outPath string, opts ...RebaseOption) error {
	data, err := os.ReadFile(filepath.Clean(inPath))
	if err != nil {
		return err
	}
	newSDR, newICCProfile, _, err := loadImageWithICC(newSDRPath)
	if err != nil {
		return err
	}
	opt := applyRebaseOptions(opts)
	opt = withICCProfile(opt, newICCProfile)
	res, err := rebaseWithOptions(data, newSDR, opt)
	if err != nil {
		return err
	}
	primaryOut, gainmapOut := outputsFromOptions(opt)
	return writeRebaseOutputs(outPath, res.Container, primaryOut, res.Primary, gainmapOut, res.Gainmap)
}

// RebaseFromEXRFile generates an UltraHDR JPEG from an SDR primary and HDR EXR input.
func RebaseFromEXRFile(primaryPath, exrPath, outPath string, opts ...RebaseOption) error {
	return rebaseUltraHDRFromHDRFile(primaryPath, exrPath, outPath, decodeEXR, opts...)
}

// RebaseFromTIFFFile generates an UltraHDR JPEG from an SDR primary and HDR TIFF input.
func RebaseFromTIFFFile(primaryPath, hdrPath, outPath string, opts ...RebaseOption) error {
	return rebaseUltraHDRFromHDRFile(primaryPath, hdrPath, outPath, decodeTIFFHDR, opts...)
}

func rebaseGainmap(oldSDR, newSDR, gainmap image.Image, meta *GainMapMetadata, oldProfile, newProfile colorProfile, workGamut colorGamut) (image.Image, error) {
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
				oldRGB := sampleSDRInProfile(oldSDR, b.Min.X+x, b.Min.Y+y, oldProfile, workGamut)
				newRGB := sampleSDRInProfile(newSDR, b.Min.X+x, b.Min.Y+y, newProfile, workGamut)
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
			oldRGB := sampleSDRInProfile(oldSDR, b.Min.X+x, b.Min.Y+y, oldProfile, workGamut)
			newRGB := sampleSDRInProfile(newSDR, b.Min.X+x, b.Min.Y+y, newProfile, workGamut)
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

func withICCProfile(opt *RebaseOptions, iccProfile []byte) *RebaseOptions {
	if len(iccProfile) == 0 {
		return opt
	}
	if opt == nil {
		return &RebaseOptions{ICCProfile: iccProfile}
	}
	if len(opt.ICCProfile) > 0 {
		return opt
	}
	local := *opt
	local.ICCProfile = iccProfile
	return &local
}

func rebaseUltraHDRFromHDRFile(primaryPath, hdrPath, outPath string, decodeHDR func([]byte) (*hdrImage, error), opts ...RebaseOption) error {
	if primaryPath == "" || hdrPath == "" || outPath == "" {
		return errors.New("missing required arguments")
	}
	newSDR, newICCProfile, primaryBytes, err := loadImageWithICC(primaryPath)
	if err != nil {
		return err
	}
	hdrBytes, err := os.ReadFile(filepath.Clean(hdrPath))
	if err != nil {
		return err
	}
	hdr, err := decodeHDR(hdrBytes)
	if err != nil {
		return err
	}

	opt := applyRebaseOptions(opts)
	opt = withICCProfile(opt, newICCProfile)
	res, err := rebaseUltraHDRFromHDR(newSDR, hdr, opt)
	if err != nil {
		return err
	}
	exif, icc, err := extractExifAndIcc(res.Primary)
	if err != nil {
		return err
	}
	if len(exif) == 0 && len(icc) == 0 {
		exif, icc, err = extractExifAndIcc(primaryBytes)
		if err != nil {
			return err
		}
	}
	secondaryISO, err := buildIsoPayload(res.Meta)
	if err != nil {
		return err
	}
	secondaryXMP := buildGainmapXMP(res.Meta)
	primaryXMP := buildPrimaryXMP(res.Meta, 0)
	container, err := assembleContainerVipsLikeWithPrimaryXMP(res.Primary, res.Gainmap, exif, icc, primaryXMP, secondaryXMP, secondaryISO)
	if err != nil {
		return err
	}
	primaryOut, gainmapOut := outputsFromOptions(opt)
	return writeRebaseOutputs(outPath, container, primaryOut, res.Primary, gainmapOut, res.Gainmap)
}

func loadImageWithICC(path string) (image.Image, []byte, []byte, error) {
	data, err := os.ReadFile(filepath.Clean(path))
	if err != nil {
		return nil, nil, nil, err
	}
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, nil, nil, err
	}
	_, icc, err := extractExifAndIcc(data)
	if err != nil {
		return nil, nil, nil, err
	}
	return img, collectICCProfile(icc), data, nil
}

func writeRebaseOutputs(outPath string, container []byte, primaryOut string, primary []byte, gainmapOut string, gainmap []byte) error {
	if err := os.WriteFile(filepath.Clean(outPath), container, 0o644); err != nil {
		return err
	}
	if primaryOut != "" {
		if err := os.WriteFile(filepath.Clean(primaryOut), primary, 0o644); err != nil {
			return err
		}
	}
	if gainmapOut != "" {
		if err := os.WriteFile(filepath.Clean(gainmapOut), gainmap, 0o644); err != nil {
			return err
		}
	}
	return nil
}

func outputsFromOptions(opt *RebaseOptions) (string, string) {
	if opt == nil {
		return "", ""
	}
	return opt.PrimaryOut, opt.GainmapOut
}
