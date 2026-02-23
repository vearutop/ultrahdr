package ultrahdr

// AssembleContainer wraps assembleContainerVipsLike for external use.
func AssembleContainer(primaryJPEG, gainmapJPEG []byte, exif []byte, icc [][]byte, secondaryXMP []byte, secondaryISO []byte) ([]byte, error) {
	return assembleContainerVipsLike(primaryJPEG, gainmapJPEG, exif, icc, secondaryXMP, secondaryISO)
}

// ExtractEXIFAndICC returns EXIF and ICC APP payloads from a JPEG.
func ExtractEXIFAndICC(jpegData []byte) ([]byte, [][]byte, error) {
	return extractExifAndIcc(jpegData)
}

// ExtractGainmapMetadataSegments returns XMP/ISO APP payloads from a gainmap JPEG.
func ExtractGainmapMetadataSegments(gainmapJPEG []byte) ([]byte, []byte, error) {
	app1, app2, err := extractAppSegments(gainmapJPEG)
	if err != nil {
		return nil, nil, err
	}
	return findXMP(app1), findISO(app2), nil
}
