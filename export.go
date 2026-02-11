package ultrahdr

// AssembleContainer wraps assembleContainerVipsLike for external use.
func AssembleContainer(primaryJPEG, gainmapJPEG []byte, exif []byte, icc [][]byte, secondaryXMP []byte, secondaryISO []byte) ([]byte, error) {
	return assembleContainerVipsLike(primaryJPEG, gainmapJPEG, exif, icc, secondaryXMP, secondaryISO)
}

// ExtractEXIFAndICC returns EXIF and ICC APP payloads from a JPEG.
func ExtractEXIFAndICC(jpegData []byte) ([]byte, [][]byte, error) {
	return extractExifAndIcc(jpegData)
}
