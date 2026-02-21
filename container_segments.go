package ultrahdr

import (
	"bytes"
	"encoding/binary"
	"errors"
	"regexp"
)

var itemLengthRe = regexp.MustCompile(`Item:Length="\d+"`)

func assembleContainerWithSegments(primaryJPEG, gainmapJPEG []byte, segs *MetadataSegments) ([]byte, error) {
	if len(primaryJPEG) < 2 || len(gainmapJPEG) < 2 {
		return nil, errors.New("invalid JPEG data")
	}

	secondaryImageSize := len(gainmapJPEG) + appSize(segs.SecondaryXMP) + appSize(segs.SecondaryISO)

	primaryXMP := segs.PrimaryXMP
	if len(primaryXMP) > 0 {
		updated, err := updatePrimaryXmpLength(primaryXMP, secondaryImageSize)
		if err != nil {
			return nil, err
		}
		primaryXMP = updated
	}

	var out bytes.Buffer
	writeSOI := func() {
		out.WriteByte(markerStart)
		out.WriteByte(markerSOI)
	}

	writeSOI()
	if len(primaryXMP) > 0 {
		writeAppSegment(&out, markerAPP1, primaryXMP)
	}
	if len(segs.PrimaryISO) > 0 {
		writeAppSegment(&out, markerAPP2, segs.PrimaryISO)
	}

	mpfLen := 2 + calculateMpfSize()
	primaryImageSize := out.Len() + mpfLen + len(primaryJPEG)
	secondaryOffset := primaryImageSize - out.Len() - 8
	mpf := generateMpf(primaryImageSize, secondaryImageSize, secondaryOffset)
	writeAppSegment(&out, markerAPP2, mpf)

	out.Write(primaryJPEG[2:])

	writeSOI()
	if len(segs.SecondaryXMP) > 0 {
		writeAppSegment(&out, markerAPP1, segs.SecondaryXMP)
	}
	if len(segs.SecondaryISO) > 0 {
		writeAppSegment(&out, markerAPP2, segs.SecondaryISO)
	}
	out.Write(gainmapJPEG[2:])

	return out.Bytes(), nil
}

// assembleContainerVipsLike mimics vips marker ordering: EXIF, ISO(version), MPF, ICC.
func assembleContainerVipsLike(primaryJPEG, gainmapJPEG []byte, exif []byte, icc [][]byte, secondaryXMP []byte, secondaryISO []byte) ([]byte, error) {
	if len(primaryJPEG) < 2 || len(gainmapJPEG) < 2 {
		return nil, errors.New("invalid JPEG data")
	}

	primaryStripped, err := stripAppSegments(primaryJPEG)
	if err != nil {
		return nil, err
	}
	gainmapStripped, err := stripAppSegments(gainmapJPEG)
	if err != nil {
		return nil, err
	}

	secondaryImageSize := len(gainmapStripped) + appSize(secondaryXMP) + appSize(secondaryISO)

	var out bytes.Buffer
	writeSOI := func() {
		out.WriteByte(markerStart)
		out.WriteByte(markerSOI)
	}

	writeSOI()
	if len(exif) > 0 {
		writeAppSegment(&out, markerAPP1, exif)
	}
	isoPrimary := secondaryISO
	if len(isoPrimary) == 0 {
		isoPrimary = buildIsoVersionOnly()
	} else if len(isoPrimary) > len(isoNamespace)+1+4 {
		// If this is full ISO metadata, keep only version (4 bytes) for primary.
		isoPrimary = append([]byte(nil), isoPrimary[:len(isoNamespace)+1+4]...)
	}

	if len(isoPrimary) > 0 {
		writeAppSegment(&out, markerAPP2, isoPrimary)
	}

	mpfLen := 2 + calculateMpfSize()
	primaryImageSize := out.Len() + mpfLen + len(primaryStripped)
	secondaryOffset := primaryImageSize - out.Len() - 8
	mpf := generateMpf(primaryImageSize, secondaryImageSize, secondaryOffset)
	writeAppSegment(&out, markerAPP2, mpf)

	for _, seg := range icc {
		writeAppSegment(&out, markerAPP2, seg)
	}

	out.Write(primaryStripped[2:])

	writeSOI()
	if len(secondaryXMP) > 0 {
		writeAppSegment(&out, markerAPP1, secondaryXMP)
	}
	if len(secondaryISO) > 0 {
		writeAppSegment(&out, markerAPP2, secondaryISO)
	}
	out.Write(gainmapStripped[2:])

	final := out.Bytes()
	if err := replaceMpfPayload(final); err != nil {
		return nil, err
	}
	return final, nil
}

// assembleContainerVipsLikeWithPrimaryXMP is like assembleContainerVipsLike, but also writes primary XMP.
func assembleContainerVipsLikeWithPrimaryXMP(primaryJPEG, gainmapJPEG []byte, exif []byte, icc [][]byte, primaryXMP []byte, secondaryXMP []byte, secondaryISO []byte) ([]byte, error) {
	if len(primaryJPEG) < 2 || len(gainmapJPEG) < 2 {
		return nil, errors.New("invalid JPEG data")
	}

	primaryStripped, err := stripAppSegments(primaryJPEG)
	if err != nil {
		return nil, err
	}
	gainmapStripped, err := stripAppSegments(gainmapJPEG)
	if err != nil {
		return nil, err
	}

	secondaryImageSize := len(gainmapStripped) + appSize(secondaryXMP) + appSize(secondaryISO)
	if len(primaryXMP) > 0 {
		updated, err := updatePrimaryXmpLength(primaryXMP, secondaryImageSize)
		if err != nil {
			return nil, err
		}
		primaryXMP = updated
	}

	var out bytes.Buffer
	writeSOI := func() {
		out.WriteByte(markerStart)
		out.WriteByte(markerSOI)
	}

	writeSOI()
	if len(exif) > 0 {
		writeAppSegment(&out, markerAPP1, exif)
	}
	if len(primaryXMP) > 0 {
		writeAppSegment(&out, markerAPP1, primaryXMP)
	}

	isoPrimary := secondaryISO
	if len(isoPrimary) == 0 {
		isoPrimary = buildIsoVersionOnly()
	} else if len(isoPrimary) > len(isoNamespace)+1+4 {
		// If this is full ISO metadata, keep only version (4 bytes) for primary.
		isoPrimary = append([]byte(nil), isoPrimary[:len(isoNamespace)+1+4]...)
	}

	if len(isoPrimary) > 0 {
		writeAppSegment(&out, markerAPP2, isoPrimary)
	}

	mpfLen := 2 + calculateMpfSize()
	primaryImageSize := out.Len() + mpfLen + len(primaryStripped)
	secondaryOffset := primaryImageSize - out.Len() - 8
	mpf := generateMpf(primaryImageSize, secondaryImageSize, secondaryOffset)
	writeAppSegment(&out, markerAPP2, mpf)

	for _, seg := range icc {
		writeAppSegment(&out, markerAPP2, seg)
	}

	out.Write(primaryStripped[2:])

	writeSOI()
	if len(secondaryXMP) > 0 {
		writeAppSegment(&out, markerAPP1, secondaryXMP)
	}
	if len(secondaryISO) > 0 {
		writeAppSegment(&out, markerAPP2, secondaryISO)
	}
	out.Write(gainmapStripped[2:])

	final := out.Bytes()
	if err := replaceMpfPayload(final); err != nil {
		return nil, err
	}
	return final, nil
}

func buildIsoVersionOnly() []byte {
	payload := append(append([]byte{}, []byte(isoNamespace)...), 0)
	payload = append(payload, 0, 0, 0, 0)
	return payload
}

// stripAppSegments removes APP0-APP15 and COM segments from a JPEG.
func stripAppSegments(jpegData []byte) ([]byte, error) {
	if len(jpegData) < 4 || jpegData[0] != markerStart || jpegData[1] != markerSOI {
		return nil, errors.New("invalid jpeg")
	}
	var out bytes.Buffer
	out.WriteByte(markerStart)
	out.WriteByte(markerSOI)
	pos := 2
	for pos+3 < len(jpegData) {
		if jpegData[pos] != markerStart {
			out.WriteByte(jpegData[pos])
			pos++
			continue
		}
		for pos < len(jpegData) && jpegData[pos] == markerStart {
			pos++
		}
		if pos >= len(jpegData) {
			break
		}
		marker := jpegData[pos]
		pos++
		if marker == markerSOS || marker == markerEOI {
			out.WriteByte(markerStart)
			out.WriteByte(marker)
			out.Write(jpegData[pos:]) // include rest
			return out.Bytes(), nil
		}
		if marker >= 0xD0 && marker <= 0xD7 {
			out.WriteByte(markerStart)
			out.WriteByte(marker)
			continue
		}
		if pos+1 >= len(jpegData) {
			return nil, errors.New("truncated marker")
		}
		segLen := int(binary.BigEndian.Uint16(jpegData[pos:]))
		if segLen < 2 || pos+segLen > len(jpegData) {
			return nil, errors.New("invalid segment length")
		}
		segStart := pos + 2
		segEnd := pos + segLen
		if marker == 0xFE || (marker >= markerAPP0 && marker <= 0xEF) {
			// skip
			pos = segEnd
			continue
		}
		// keep other markers
		out.WriteByte(markerStart)
		out.WriteByte(marker)
		out.Write(jpegData[pos : pos+2]) // length
		out.Write(jpegData[segStart:segEnd])
		pos = segEnd
	}
	return out.Bytes(), nil
}

func replaceMpfPayload(data []byte) error {
	// Find MPF segment start (payload start) and length.
	mpfStart := -1
	mpfLen := -1
	for i := 2; i+3 < len(data); {
		if data[i] != 0xFF {
			i++
			continue
		}
		for i < len(data) && data[i] == 0xFF {
			i++
		}
		if i >= len(data) {
			break
		}
		marker := data[i]
		i++
		if marker == 0xDA || marker == 0xD9 {
			break
		}
		if marker >= 0xD0 && marker <= 0xD7 {
			continue
		}
		if i+1 >= len(data) {
			return errors.New("truncated marker")
		}
		segLen := int(binary.BigEndian.Uint16(data[i:]))
		segStart := i + 2
		segEnd := i + segLen
		if marker == 0xE2 && segEnd <= len(data) && bytes.HasPrefix(data[segStart:segEnd], mpfSig) {
			mpfStart = segStart
			mpfLen = segEnd - segStart
			break
		}
		i = segEnd
	}
	if mpfStart < 0 || mpfLen <= 0 {
		return errors.New("mpf not found")
	}

	// Find JPEG ranges.
	ranges, err := scanJPEGs(data)
	if err != nil || len(ranges) < 2 {
		return errors.New("jpeg ranges not found")
	}
	primarySize := ranges[0][1] - ranges[0][0]
	secondarySize := ranges[1][1] - ranges[1][0]
	secondaryOffset := ranges[1][0] - (mpfStart + 4) // relative to TIFF header

	newMpf := generateMpf(primarySize, secondarySize, secondaryOffset)
	if len(newMpf) != mpfLen {
		return errors.New("mpf size mismatch")
	}
	copy(data[mpfStart:mpfStart+mpfLen], newMpf)
	return nil
}

func updatePrimaryXmpLength(payload []byte, newLen int) ([]byte, error) {
	idx := bytes.Index(payload, []byte(xmpNamespace))
	if idx == -1 {
		return nil, errors.New("primary xmp namespace missing")
	}
	// Replace Item:Length="..." in XML portion
	str := string(payload)
	repl := itemLengthRe.ReplaceAllString(str, "Item:Length=\""+itoa(newLen)+"\"")
	if repl == str {
		return payload, nil
	}
	return []byte(repl), nil
}

func appSize(payload []byte) int {
	if len(payload) == 0 {
		return 0
	}
	return 4 + len(payload)
}

func itoa(v int) string {
	if v == 0 {
		return "0"
	}
	neg := v < 0
	if neg {
		v = -v
	}
	buf := [32]byte{}
	i := len(buf)
	for v > 0 {
		i--
		buf[i] = byte('0' + v%10)
		v /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}
