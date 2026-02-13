package ultrahdr

import (
	"bytes"
	"sort"
)

type colorGamut int

type colorTransfer int

const (
	colorGamutSRGB colorGamut = iota
	colorGamutDisplayP3
	colorGamutAdobeRGB
)

const (
	colorTransferSRGB colorTransfer = iota
	colorTransferGamma22
)

type colorProfile struct {
	gamut    colorGamut
	transfer colorTransfer
}

func detectColorProfileFromICCProfile(profile []byte) colorProfile {
	if len(profile) == 0 {
		return colorProfile{gamut: colorGamutSRGB, transfer: colorTransferSRGB}
	}
	lower := bytes.ToLower(profile)
	// Simple heuristic: enough for common camera/jpeg workflows.
	if bytes.Contains(lower, []byte("display p3")) || bytes.Contains(lower, []byte("dci-p3")) {
		return colorProfile{gamut: colorGamutDisplayP3, transfer: colorTransferSRGB}
	}
	if bytes.Contains(lower, []byte("adobe rgb")) || bytes.Contains(lower, []byte("adobergb")) {
		return colorProfile{gamut: colorGamutAdobeRGB, transfer: colorTransferGamma22}
	}
	return colorProfile{gamut: colorGamutSRGB, transfer: colorTransferSRGB}
}

func collectICCProfile(icc [][]byte) []byte {
	type chunk struct {
		seq  int
		data []byte
	}
	chunks := make([]chunk, 0, len(icc))
	for _, p := range icc {
		// ICC APP2 payload: "ICC_PROFILE\0" + seq + total + profile bytes.
		if len(p) > len(iccSig)+2 && bytes.HasPrefix(p, iccSig) {
			chunks = append(chunks, chunk{seq: int(p[len(iccSig)]), data: append([]byte(nil), p[len(iccSig)+2:]...)})
		}
	}
	if len(chunks) == 0 {
		return nil
	}
	sort.Slice(chunks, func(i, j int) bool { return chunks[i].seq < chunks[j].seq })
	total := 0
	for _, c := range chunks {
		total += len(c.data)
	}
	out := make([]byte, 0, total)
	for _, c := range chunks {
		out = append(out, c.data...)
	}
	return out
}

func convertLinearGamut(v rgb, from, to colorGamut) rgb {
	if from == to {
		return v
	}
	// Matrices are D65 linear RGB <-> XYZ.
	x, y, z := rgbToXYZ(v, from)
	return xyzToRGB(x, y, z, to)
}

func rgbToXYZ(v rgb, from colorGamut) (float32, float32, float32) {
	switch from {
	case colorGamutDisplayP3:
		return 0.48657095*v.r + 0.2656677*v.g + 0.19821729*v.b,
			0.22897457*v.r + 0.69173855*v.g + 0.07928691*v.b,
			0.04511338*v.g + 1.0439444*v.b
	case colorGamutAdobeRGB:
		return 0.5767309*v.r + 0.185554*v.g + 0.1881852*v.b,
			0.2973769*v.r + 0.6273491*v.g + 0.0752741*v.b,
			0.0270343*v.r + 0.0706872*v.g + 0.9911085*v.b
	default:
		return 0.4123908*v.r + 0.35758433*v.g + 0.1804808*v.b,
			0.212639*v.r + 0.71516865*v.g + 0.07219232*v.b,
			0.019330818*v.r + 0.11919478*v.g + 0.95053214*v.b
	}
}

func xyzToRGB(x, y, z float32, to colorGamut) rgb {
	switch to {
	case colorGamutDisplayP3:
		return rgb{
			r: 2.493497*x - 0.9313836*y - 0.4027108*z,
			g: -0.829489*x + 1.7626641*y + 0.023624685*z,
			b: 0.03584583*x - 0.07617239*y + 0.9568845*z,
		}
	case colorGamutAdobeRGB:
		return rgb{
			r: 2.041369*x - 0.5649464*y - 0.3446944*z,
			g: -0.969266*x + 1.8760108*y + 0.041556*z,
			b: 0.0134474*x - 0.1183897*y + 1.0154096*z,
		}
	default:
		return rgb{
			r: 3.24097*x - 1.5373832*y - 0.49861076*z,
			g: -0.96924365*x + 1.8759675*y + 0.041555058*z,
			b: 0.05563008*x - 0.20397696*y + 1.0569715*z,
		}
	}
}
