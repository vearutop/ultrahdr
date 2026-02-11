package ultrahdr

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
