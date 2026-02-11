package ultrahdr

import "errors"

const metadataBundleFormat = "ultrahdr-meta-1"

// MetadataBundle captures the metadata needed to reassemble an UltraHDR container.
// Byte fields are base64-encoded in JSON.
type MetadataBundle struct {
	Format       string   `json:"format"`
	PrimaryXMP   []byte   `json:"primary_xmp,omitempty"`
	PrimaryISO   []byte   `json:"primary_iso,omitempty"`
	SecondaryXMP []byte   `json:"secondary_xmp,omitempty"`
	SecondaryISO []byte   `json:"secondary_iso,omitempty"`
	Exif         []byte   `json:"exif,omitempty"`
	ICC          [][]byte `json:"icc,omitempty"`
}

// BuildMetadataBundle builds a metadata bundle from split segments and primary JPEG.
func BuildMetadataBundle(primaryJPEG []byte, segs *MetadataSegments) (*MetadataBundle, error) {
	if segs == nil {
		return nil, errors.New("metadata segments missing")
	}
	exif, icc, err := extractExifAndIcc(primaryJPEG)
	if err != nil {
		return nil, err
	}
	return &MetadataBundle{
		Format:       metadataBundleFormat,
		PrimaryXMP:   segs.PrimaryXMP,
		PrimaryISO:   segs.PrimaryISO,
		SecondaryXMP: segs.SecondaryXMP,
		SecondaryISO: segs.SecondaryISO,
		Exif:         exif,
		ICC:          icc,
	}, nil
}

// Validate ensures the bundle has the required fields to build a container.
func (b *MetadataBundle) Validate() error {
	if b == nil {
		return errors.New("metadata bundle is nil")
	}
	if b.Format == "" {
		return errors.New("metadata bundle missing format")
	}
	if b.Format != metadataBundleFormat {
		return errors.New("unsupported metadata bundle format")
	}
	if len(b.SecondaryXMP) == 0 && len(b.SecondaryISO) == 0 {
		return errors.New("metadata bundle missing gainmap metadata")
	}
	return nil
}

// AssembleFromBundle builds a container using metadata from the bundle.
func AssembleFromBundle(primaryJPEG, gainmapJPEG []byte, b *MetadataBundle) ([]byte, error) {
	if err := b.Validate(); err != nil {
		return nil, err
	}
	return assembleContainerVipsLike(primaryJPEG, gainmapJPEG, b.Exif, b.ICC, b.SecondaryXMP, b.SecondaryISO)
}
