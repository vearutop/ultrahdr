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

// JoinWithBoost assembles an UltraHDR container from a plain primary JPEG and a plain
// gainmap JPEG (no embedded gainmap metadata), using maxBoost as the content boost
// reached by white gainmap pixels (e.g. 4 means +2 stops).
func JoinWithBoost(primaryJPEG, gainmapJPEG []byte, maxBoost float32) ([]byte, error) {
	if len(primaryJPEG) == 0 || len(gainmapJPEG) == 0 {
		return nil, errors.New("missing primary or gainmap JPEG")
	}
	if maxBoost <= 1 {
		return nil, errors.New("boost must be greater than 1")
	}
	meta := &GainMapMetadata{
		Version:         jpegrVersion,
		MaxContentBoost: [3]float32{maxBoost, maxBoost, maxBoost},
		MinContentBoost: [3]float32{1, 1, 1},
		Gamma:           [3]float32{1, 1, 1},
		OffsetSDR:       [3]float32{1.0 / 64, 1.0 / 64, 1.0 / 64},
		OffsetHDR:       [3]float32{1.0 / 64, 1.0 / 64, 1.0 / 64},
		HDRCapacityMin:  1,
		HDRCapacityMax:  maxBoost,
	}
	exif, icc, err := extractExifAndIcc(primaryJPEG)
	if err != nil {
		return nil, err
	}
	iso, err := buildIsoPayload(meta)
	if err != nil {
		return nil, err
	}
	return assembleContainerVipsLikeWithPrimaryXMP(primaryJPEG, gainmapJPEG, exif, icc, buildPrimaryXMP(meta, 0), buildGainmapXMP(meta), iso)
}
