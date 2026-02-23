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
func (r *Result) BuildMetadataBundle() (*MetadataBundle, error) {
	if r == nil {
		return nil, errors.New("result is nil")
	}
	if r.Segs == nil {
		return nil, errors.New("metadata segments missing")
	}
	exif, icc, err := extractExifAndIcc(r.Primary)
	if err != nil {
		return nil, err
	}
	return &MetadataBundle{
		Format:       metadataBundleFormat,
		PrimaryXMP:   r.Segs.PrimaryXMP,
		PrimaryISO:   r.Segs.PrimaryISO,
		SecondaryXMP: r.Segs.SecondaryXMP,
		SecondaryISO: r.Segs.SecondaryISO,
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

// assembleFromBundle builds a container using metadata from the bundle.
func assembleFromBundle(primaryJPEG, gainmapJPEG []byte, b *MetadataBundle) ([]byte, error) {
	if err := b.Validate(); err != nil {
		return nil, err
	}
	return assembleContainerVipsLike(primaryJPEG, gainmapJPEG, b.Exif, b.ICC, b.SecondaryXMP, b.SecondaryISO)
}
