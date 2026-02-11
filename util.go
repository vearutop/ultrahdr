package ultrahdr

import "math"

func clamp(v, min, max float32) float32 {
	if v < min {
		return min
	}
	if v > max {
		return max
	}
	return v
}

func log2f(v float32) float32 { return float32(math.Log2(float64(v))) }
func exp2f(v float32) float32 { return float32(math.Exp2(float64(v))) }

func srgbInvOetf(v float32) float32 {
	if v <= 0.04045 {
		return v / 12.92
	}
	return float32(math.Pow(float64((v+0.055)/1.055), 2.4))
}

func roundf(v float32) float32 {
	return float32(math.Round(float64(v)))
}
