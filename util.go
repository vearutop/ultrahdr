package ultrahdr

import "math"

func log2f(v float32) float32 { return float32(math.Log2(float64(v))) }
func exp2f(v float32) float32 { return float32(math.Exp2(float64(v))) }

func srgbInvOetf(v float32) float32 {
	if v <= 0.04045 {
		return v / 12.92
	}
	return float32(math.Pow(float64((v+0.055)/1.055), 2.4))
}

func srgbOetf(v float32) float32 {
	if v <= 0.0031308 {
		return 12.92 * v
	}
	return 1.055*float32(math.Pow(float64(v), 1.0/2.4)) - 0.055
}
