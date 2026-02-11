package ultrahdr

// AssembleContainerVipsLike wraps assembleContainerVipsLike for external use.
func AssembleContainerVipsLike(primaryJPEG, gainmapJPEG []byte, exif []byte, icc [][]byte, secondaryXMP []byte, secondaryISO []byte) ([]byte, error) {
	return assembleContainerVipsLike(primaryJPEG, gainmapJPEG, exif, icc, secondaryXMP, secondaryISO)
}

// ExtractExifAndIcc returns EXIF and ICC APP payloads from a JPEG.
func ExtractExifAndIcc(jpegData []byte) ([]byte, [][]byte, error) {
	return extractExifAndIcc(jpegData)
}

// MetadataBundleFormat exposes the current metadata bundle format identifier.
func MetadataBundleFormat() string {
	return metadataBundleFormat
}
