package ultrahdr

import "errors"

// Join assembles an UltraHDR container from primary and gainmap JPEGs.
// If bundle is provided, it is used as the metadata source. If template is provided,
// it is used to build the bundle. Otherwise gainmap metadata is extracted from the
// gainmap JPEG and EXIF/ICC are extracted from the primary JPEG.
func Join(primaryJPEG, gainmapJPEG []byte, bundle *MetadataBundle, template *Result) ([]byte, error) {
	if len(primaryJPEG) == 0 || len(gainmapJPEG) == 0 {
		return nil, errors.New("missing primary or gainmap JPEG")
	}
	if bundle != nil {
		return assembleFromBundle(primaryJPEG, gainmapJPEG, bundle)
	}
	if template != nil {
		bundle, err := template.BuildMetadataBundle()
		if err != nil {
			return nil, err
		}
		return assembleFromBundle(primaryJPEG, gainmapJPEG, bundle)
	}

	exif, icc, err := extractExifAndIcc(primaryJPEG)
	if err != nil {
		return nil, err
	}
	if len(exif) == 0 && len(icc) == 0 {
		exif, icc, err = extractExifAndIcc(gainmapJPEG)
		if err != nil {
			return nil, err
		}
	}

	app1, app2, err := extractAppSegments(gainmapJPEG)
	if err != nil {
		return nil, err
	}
	secondaryXMP := findXMP(app1)
	secondaryISO := findISO(app2)

	return assembleContainerVipsLike(primaryJPEG, gainmapJPEG, exif, icc, secondaryXMP, secondaryISO)
}
