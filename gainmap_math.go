package ultrahdr

import "math"

type rgb struct {
	r, g, b float32
}

func encodeGain(sdr, hdr float32, meta *GainMapMetadata, log2Min, log2Max float32, idx int) uint8 {
	gain := float32(1.0)
	if sdr > 0 {
		gain = hdr / sdr
	}
	if gain < meta.MinContentBoost[idx] {
		gain = meta.MinContentBoost[idx]
	}
	if gain > meta.MaxContentBoost[idx] {
		gain = meta.MaxContentBoost[idx]
	}
	gainNorm := (log2f(gain) - log2Min) / (log2Max - log2Min)
	if meta.Gamma[idx] != 1 {
		gainNorm = float32(math.Pow(float64(gainNorm), float64(meta.Gamma[idx])))
	}
	val := gainNorm * 255.0
	if val < 0 {
		val = 0
	}
	if val > 255 {
		val = 255
	}
	return uint8(val + 0.5)
}

func applyGainSingle(e rgb, gain float32, meta *GainMapMetadata, weight float32) rgb {
	if meta.Gamma[0] != 1 {
		gain = float32(math.Pow(float64(gain), float64(1.0/meta.Gamma[0])))
	}
	logBoost := log2f(meta.MinContentBoost[0])*(1.0-gain) + log2f(meta.MaxContentBoost[0])*gain
	gainFactor := exp2f(logBoost * weight)
	return rgb{
		r: (e.r+meta.OffsetSDR[0])*gainFactor - meta.OffsetHDR[0],
		g: (e.g+meta.OffsetSDR[0])*gainFactor - meta.OffsetHDR[0],
		b: (e.b+meta.OffsetSDR[0])*gainFactor - meta.OffsetHDR[0],
	}
}

func applyGainRGB(e rgb, gain rgb, meta *GainMapMetadata, weight float32) rgb {
	if meta.Gamma[0] != 1 {
		gain.r = float32(math.Pow(float64(gain.r), float64(1.0/meta.Gamma[0])))
	}
	if meta.Gamma[1] != 1 {
		gain.g = float32(math.Pow(float64(gain.g), float64(1.0/meta.Gamma[1])))
	}
	if meta.Gamma[2] != 1 {
		gain.b = float32(math.Pow(float64(gain.b), float64(1.0/meta.Gamma[2])))
	}
	logBoostR := log2f(meta.MinContentBoost[0])*(1.0-gain.r) + log2f(meta.MaxContentBoost[0])*gain.r
	logBoostG := log2f(meta.MinContentBoost[1])*(1.0-gain.g) + log2f(meta.MaxContentBoost[1])*gain.g
	logBoostB := log2f(meta.MinContentBoost[2])*(1.0-gain.b) + log2f(meta.MaxContentBoost[2])*gain.b
	gainFactorR := exp2f(logBoostR * weight)
	gainFactorG := exp2f(logBoostG * weight)
	gainFactorB := exp2f(logBoostB * weight)
	return rgb{
		r: (e.r+meta.OffsetSDR[0])*gainFactorR - meta.OffsetHDR[0],
		g: (e.g+meta.OffsetSDR[1])*gainFactorG - meta.OffsetHDR[1],
		b: (e.b+meta.OffsetSDR[2])*gainFactorB - meta.OffsetHDR[2],
	}
}
