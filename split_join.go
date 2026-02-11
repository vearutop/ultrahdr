package ultrahdr

import "errors"

type SplitResult struct {
	PrimaryJPEG, GainmapJPEG []byte
	Meta                     *GainMapMetadata
	Segs                     *MetadataSegments
}

// Split extracts primary/gainmap JPEGs, metadata, and raw XMP/ISO segments.
func Split(data []byte) (*SplitResult, error) {
	ranges, err := scanJPEGs(data)
	if err != nil {
		return nil, err
	}

	res := SplitResult{}

	if len(ranges) < 2 {
		return nil, errors.New("gainmap image not found")
	}
	res.PrimaryJPEG = append([]byte(nil), data[ranges[0][0]:ranges[0][1]]...)
	res.GainmapJPEG = append([]byte(nil), data[ranges[1][0]:ranges[1][1]]...)

	res.Segs = &MetadataSegments{}
	hApp1, hApp2, err := extractContainerHeaderSegments(data)
	if err != nil {
		return nil, err
	}
	res.Segs.PrimaryXMP = findXMP(hApp1)
	res.Segs.PrimaryISO = findISO(hApp2)

	gApp1, gApp2, err := extractAppSegments(res.GainmapJPEG)
	if err != nil {
		return nil, err
	}
	res.Segs.SecondaryXMP = findXMP(gApp1)
	res.Segs.SecondaryISO = findISO(gApp2)

	if iso := res.Segs.SecondaryISO; iso != nil {
		payload := iso[len(isoNamespace)+1:]
		res.Meta, err = decodeGainmapMetadataISO(payload)
		if err != nil {
			return nil, err
		}
		return &res, nil
	}
	if xmp := res.Segs.SecondaryXMP; xmp != nil {
		res.Meta, err = parseXMP(xmp)
		if err != nil {
			return nil, err
		}
		return &res, nil
	}
	return nil, errors.New("no gainmap metadata found")
}

// Join assembles a JPEG/R container using raw metadata segments.
// PrimaryXMP is updated to reflect the new gainmap length.
func (sr SplitResult) Join() ([]byte, error) {
	if sr.Segs == nil {
		return nil, errors.New("segments required")
	}
	return assembleContainerWithSegments(sr.PrimaryJPEG, sr.GainmapJPEG, sr.Segs)
}
