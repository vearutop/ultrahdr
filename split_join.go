package ultrahdr

import "errors"

// Split extracts the primary and gainmap JPEG images and metadata from a JPEG/R container.
func Split(data []byte) (primaryJPEG []byte, gainmapJPEG []byte, meta *GainMapMetadata, err error) {
	ranges, err := scanJPEGs(data)
	if err != nil {
		return nil, nil, nil, err
	}
	if len(ranges) < 2 {
		return nil, nil, nil, errors.New("gainmap image not found")
	}
	primaryJPEG = append([]byte(nil), data[ranges[0][0]:ranges[0][1]]...)
	gainmapJPEG = append([]byte(nil), data[ranges[1][0]:ranges[1][1]]...)

	app1, app2, err := extractAppSegments(gainmapJPEG)
	if err != nil {
		return nil, nil, nil, err
	}
	if iso := findISO(app2); iso != nil {
		payload := iso[len(isoNamespace)+1:]
		meta, err = decodeGainmapMetadataISO(payload)
		if err != nil {
			return nil, nil, nil, err
		}
		return primaryJPEG, gainmapJPEG, meta, nil
	}
	if xmp := findXMP(app1); xmp != nil {
		meta, err = parseXMP(xmp)
		if err != nil {
			return nil, nil, nil, err
		}
		return primaryJPEG, gainmapJPEG, meta, nil
	}
	return nil, nil, nil, errors.New("no gainmap metadata found")
}

// SplitWithSegments extracts primary/gainmap JPEGs, metadata, and raw XMP/ISO segments.
func SplitWithSegments(data []byte) (primaryJPEG []byte, gainmapJPEG []byte, meta *GainMapMetadata, segs *MetadataSegments, err error) {
	ranges, err := scanJPEGs(data)
	if err != nil {
		return nil, nil, nil, nil, err
	}
	if len(ranges) < 2 {
		return nil, nil, nil, nil, errors.New("gainmap image not found")
	}
	primaryJPEG = append([]byte(nil), data[ranges[0][0]:ranges[0][1]]...)
	gainmapJPEG = append([]byte(nil), data[ranges[1][0]:ranges[1][1]]...)

	segs = &MetadataSegments{}
	hApp1, hApp2, err := extractContainerHeaderSegments(data)
	if err != nil {
		return nil, nil, nil, nil, err
	}
	segs.PrimaryXMP = findXMP(hApp1)
	segs.PrimaryISO = findISO(hApp2)

	gApp1, gApp2, err := extractAppSegments(gainmapJPEG)
	if err != nil {
		return nil, nil, nil, nil, err
	}
	segs.SecondaryXMP = findXMP(gApp1)
	segs.SecondaryISO = findISO(gApp2)

	if iso := segs.SecondaryISO; iso != nil {
		payload := iso[len(isoNamespace)+1:]
		meta, err = decodeGainmapMetadataISO(payload)
		if err != nil {
			return nil, nil, nil, nil, err
		}
		return primaryJPEG, gainmapJPEG, meta, segs, nil
	}
	if xmp := segs.SecondaryXMP; xmp != nil {
		meta, err = parseXMP(xmp)
		if err != nil {
			return nil, nil, nil, nil, err
		}
		return primaryJPEG, gainmapJPEG, meta, segs, nil
	}
	return nil, nil, nil, nil, errors.New("no gainmap metadata found")
}

// Join assembles a JPEG/R container from primary and gainmap JPEG images and metadata.
func Join(primaryJPEG, gainmapJPEG []byte, meta *GainMapMetadata) ([]byte, error) {
	if meta == nil {
		return nil, errors.New("metadata required")
	}
	return assembleContainer(primaryJPEG, gainmapJPEG, meta)
}

// JoinWithSegments assembles a JPEG/R container using raw metadata segments.
// PrimaryXMP is updated to reflect the new gainmap length.
func JoinWithSegments(primaryJPEG, gainmapJPEG []byte, segs *MetadataSegments) ([]byte, error) {
	if segs == nil {
		return nil, errors.New("segments required")
	}
	return assembleContainerWithSegments(primaryJPEG, gainmapJPEG, segs)
}
