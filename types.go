package ultrahdr

// ColorGamut identifies a supported color gamut.
type ColorGamut int

const (
	GamutUnspecified ColorGamut = iota
	GamutBT709
	GamutDisplayP3
	GamutBT2100
)

// ColorTransfer identifies a supported transfer function.
type ColorTransfer int

const (
	TransferUnspecified ColorTransfer = iota
	TransferSRGB
	TransferLinear
	TransferPQ
	TransferHLG
)

// HDRImage stores a linear-light HDR image in RGB float32.
// Pixel values are expected to be relative to SDR white (1.0 = SDR white).
type HDRImage struct {
	Width  int
	Height int
	Stride int // pixels per row, in RGB triplets
	Pix    []float32
	Gamut  ColorGamut
	// Transfer describes how Pix values should be interpreted if not linear.
	// For now, the implementation assumes linear and ignores other values.
	Transfer ColorTransfer
}

// GainMapMetadata corresponds to the float metadata in the C++ library.
type GainMapMetadata struct {
	Version         string
	MaxContentBoost [3]float32
	MinContentBoost [3]float32
	Gamma           [3]float32
	OffsetSDR       [3]float32
	OffsetHDR       [3]float32
	HDRCapacityMin  float32
	HDRCapacityMax  float32
	UseBaseCG       bool
}

// MetadataSegments holds raw APP payloads for XMP/ISO blocks.
// These payloads include the namespace prefix and null terminator.
type MetadataSegments struct {
	PrimaryXMP   []byte
	PrimaryISO   []byte
	SecondaryXMP []byte
	SecondaryISO []byte
}

// EncodeOptions controls JPEG/R encoding.
type EncodeOptions struct {
	Quality           int     // base JPEG quality (0-100)
	GainMapQuality    int     // gainmap JPEG quality (0-100)
	GainMapScale      int     // downscale factor for gainmap (>=1)
	UseMultiChannelGM bool    // use RGB gainmap instead of luma
	Gamma             float32 // gainmap gamma
	HDRWhiteNits      float32 // reference HDR white in nits (default 1000)
	TargetDisplayNits float32 // optional, if >0 sets HDRCapacityMax
	UseLuminance      bool    // use luminance instead of max(rgb) for gainmap
}

// DecodeOptions controls JPEG/R decoding.
type DecodeOptions struct {
	MaxDisplayBoost float32 // maximum display boost, >=1; if 0 uses metadata HDRCapacityMax
}
