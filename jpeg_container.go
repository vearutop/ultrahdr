package ultrahdr

import (
	"bytes"
	"encoding/binary"
	"errors"
	"sort"
)

const (
	markerStart = 0xFF
	markerSOI   = 0xD8
	markerEOI   = 0xD9
	markerSOS   = 0xDA
	markerAPP0  = 0xE0
	markerAPP1  = 0xE1
	markerAPP2  = 0xE2
)

const (
	xmpNamespace = "http://ns.adobe.com/xap/1.0/"
	isoNamespace = "urn:iso:std:iso:ts:21496:-1"
)

var (
	exifSig = []byte{'E', 'x', 'i', 'f', 0, 0}
	iccSig  = []byte{'I', 'C', 'C', '_', 'P', 'R', 'O', 'F', 'I', 'L', 'E', 0}
)

func scanJPEGs(data []byte) ([][2]int, error) {
	var ranges [][2]int
	i := 0
	for i+1 < len(data) {
		if data[i] == markerStart && data[i+1] == markerSOI {
			start := i
			end, err := findJPEGEnd(data, i)
			if err != nil {
				return nil, err
			}
			ranges = append(ranges, [2]int{start, end})
			i = end
			continue
		}
		i++
	}
	if len(ranges) == 0 {
		return nil, errors.New("no JPEG images found")
	}
	return ranges, nil
}

func findJPEGEnd(data []byte, start int) (int, error) {
	if start+1 >= len(data) || data[start] != markerStart || data[start+1] != markerSOI {
		return 0, errors.New("not a JPEG SOI")
	}
	pos := start + 2
	inScan := false
	for pos+1 < len(data) {
		if !inScan {
			if data[pos] != markerStart {
				pos++
				continue
			}
			for pos < len(data) && data[pos] == markerStart {
				pos++
			}
			if pos >= len(data) {
				break
			}
			marker := data[pos]
			pos++
			switch marker {
			case markerSOI:
				continue
			case markerEOI:
				return pos, nil
			case markerSOS:
				if pos+1 >= len(data) {
					return 0, errors.New("truncated SOS")
				}
				segLen := int(binary.BigEndian.Uint16(data[pos:]))
				pos += segLen
				inScan = true
				continue
			}
			if marker >= 0xD0 && marker <= 0xD7 {
				continue
			}
			if marker == 0x01 {
				continue
			}
			if pos+1 >= len(data) {
				return 0, errors.New("truncated marker segment")
			}
			segLen := int(binary.BigEndian.Uint16(data[pos:]))
			if segLen < 2 {
				return 0, errors.New("invalid marker length")
			}
			pos += segLen
			continue
		}

		// in scan data
		if data[pos] == markerStart {
			if pos+1 >= len(data) {
				return 0, errors.New("truncated scan data")
			}
			next := data[pos+1]
			switch {
			case next == 0x00:
				pos += 2
				continue
			case next >= 0xD0 && next <= 0xD7:
				pos += 2
				continue
			case next == markerEOI:
				return pos + 2, nil
			default:
				// Attempt to parse marker within scan data.
				pos += 2
				if pos+1 >= len(data) {
					return 0, errors.New("truncated marker in scan")
				}
				segLen := int(binary.BigEndian.Uint16(data[pos:]))
				if segLen < 2 {
					return 0, errors.New("invalid marker length in scan")
				}
				pos += segLen
				continue
			}
		}
		pos++
	}
	return 0, errors.New("no EOI found")
}

func extractAppSegments(jpegData []byte) (app1 [][]byte, app2 [][]byte, err error) {
	if len(jpegData) < 4 || jpegData[0] != markerStart || jpegData[1] != markerSOI {
		return nil, nil, errors.New("invalid JPEG")
	}
	pos := 2
	for pos+3 < len(jpegData) {
		if jpegData[pos] != markerStart {
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
			break
		}
		if marker >= 0xD0 && marker <= 0xD7 {
			continue
		}
		if pos+1 >= len(jpegData) {
			return nil, nil, errors.New("truncated marker")
		}
		segLen := int(binary.BigEndian.Uint16(jpegData[pos:]))
		if segLen < 2 || pos+segLen > len(jpegData) {
			return nil, nil, errors.New("invalid segment length")
		}
		segStart := pos + 2
		segEnd := pos + segLen
		switch marker {
		case markerAPP1:
			app1 = append(app1, append([]byte(nil), jpegData[segStart:segEnd]...))
		case markerAPP2:
			app2 = append(app2, append([]byte(nil), jpegData[segStart:segEnd]...))
		}
		pos = segEnd
	}
	return app1, app2, nil
}

// extractContainerHeaderSegments returns APP1/APP2 payloads in the container header up to MPF.
func extractContainerHeaderSegments(data []byte) (app1 [][]byte, app2 [][]byte, err error) {
	if len(data) < 4 || data[0] != markerStart || data[1] != markerSOI {
		return nil, nil, errors.New("invalid jpeg")
	}
	pos := 2
	for pos+3 < len(data) {
		if data[pos] != markerStart {
			pos++
			continue
		}
		for pos < len(data) && data[pos] == markerStart {
			pos++
		}
		if pos >= len(data) {
			break
		}
		marker := data[pos]
		pos++
		if marker == markerSOS || marker == markerEOI {
			break
		}
		if marker >= 0xD0 && marker <= 0xD7 {
			continue
		}
		if pos+1 >= len(data) {
			return nil, nil, errors.New("truncated marker")
		}
		segLen := int(binary.BigEndian.Uint16(data[pos:]))
		if segLen < 2 || pos+segLen > len(data) {
			return nil, nil, errors.New("invalid segment length")
		}
		segStart := pos + 2
		segEnd := pos + segLen
		payload := append([]byte(nil), data[segStart:segEnd]...)
		switch marker {
		case markerAPP1:
			app1 = append(app1, payload)
		case markerAPP2:
			app2 = append(app2, payload)
			if bytes.HasPrefix(payload, mpfSig) {
				return app1, app2, nil
			}
		}
		pos = segEnd
	}
	return app1, app2, nil
}

func findXMP(app1 [][]byte) []byte {
	for _, seg := range app1 {
		if bytes.HasPrefix(seg, append([]byte(xmpNamespace), 0)) {
			return seg
		}
	}
	return nil
}

func findISO(app2 [][]byte) []byte {
	for _, seg := range app2 {
		if bytes.HasPrefix(seg, append([]byte(isoNamespace), 0)) {
			return seg
		}
	}
	return nil
}

type iccSegment struct {
	seq  int
	data []byte
}

type appSegment struct {
	marker  byte
	payload []byte
}

// extractExifAndIcc returns the EXIF APP1 payload (if present) and ICC APP2 payloads.
func extractExifAndIcc(jpegData []byte) ([]byte, [][]byte, error) {
	app1, app2, err := extractAppSegments(jpegData)
	if err != nil {
		return nil, nil, err
	}
	var exif []byte
	for _, seg := range app1 {
		if bytes.HasPrefix(seg, exifSig) {
			exif = append([]byte(nil), seg...)
			break
		}
	}
	var iccSegs []iccSegment
	for _, seg := range app2 {
		if bytes.HasPrefix(seg, iccSig) && len(seg) >= len(iccSig)+2 {
			seq := int(seg[len(iccSig)])
			iccSegs = append(iccSegs, iccSegment{seq: seq, data: append([]byte(nil), seg...)})
		}
	}
	if len(iccSegs) == 0 {
		return exif, nil, nil
	}
	sort.Slice(iccSegs, func(i, j int) bool { return iccSegs[i].seq < iccSegs[j].seq })
	out := make([][]byte, 0, len(iccSegs))
	for _, s := range iccSegs {
		out = append(out, s.data)
	}
	return exif, out, nil
}

// insertExifIcc builds a new JPEG by inserting EXIF and ICC segments after SOI.
func insertExifIcc(jpegData []byte, exif []byte, icc [][]byte) ([]byte, error) {
	if len(jpegData) < 2 || jpegData[0] != markerStart || jpegData[1] != markerSOI {
		return nil, errors.New("invalid jpeg")
	}
	var out bytes.Buffer
	out.WriteByte(markerStart)
	out.WriteByte(markerSOI)
	if len(exif) > 0 {
		writeAppSegment(&out, markerAPP1, exif)
	}
	for _, seg := range icc {
		writeAppSegment(&out, markerAPP2, seg)
	}
	out.Write(jpegData[2:])
	return out.Bytes(), nil
}

func writeAppSegment(out *bytes.Buffer, marker byte, payload []byte) {
	out.WriteByte(markerStart)
	out.WriteByte(marker)
	length := uint16(len(payload) + 2)
	out.WriteByte(byte(length >> 8))
	out.WriteByte(byte(length))
	out.Write(payload)
}

// extractAppSegmentsAll returns APP0-APP15 and COM segments in order.
func extractAppSegmentsAll(jpegData []byte) ([]appSegment, error) {
	if len(jpegData) < 4 || jpegData[0] != markerStart || jpegData[1] != markerSOI {
		return nil, errors.New("invalid JPEG")
	}
	var segs []appSegment
	pos := 2
	for pos+3 < len(jpegData) {
		if jpegData[pos] != markerStart {
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
			break
		}
		if marker >= 0xD0 && marker <= 0xD7 {
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
			payload := append([]byte(nil), jpegData[segStart:segEnd]...)
			segs = append(segs, appSegment{marker: marker, payload: payload})
		}
		pos = segEnd
	}
	return segs, nil
}

// filterPreserveAppSegments removes XMP/ISO metadata segments that are handled at container level.
func filterPreserveAppSegments(segs []appSegment) []appSegment {
	out := make([]appSegment, 0, len(segs))
	for _, s := range segs {
		if s.marker == markerAPP1 && bytes.HasPrefix(s.payload, append([]byte(xmpNamespace), 0)) {
			continue
		}
		if s.marker == markerAPP2 && bytes.HasPrefix(s.payload, append([]byte(isoNamespace), 0)) {
			continue
		}
		out = append(out, s)
	}
	return out
}

// insertAppSegments inserts APP segments after SOI.
func insertAppSegments(jpegData []byte, segs []appSegment) ([]byte, error) {
	if len(jpegData) < 2 || jpegData[0] != markerStart || jpegData[1] != markerSOI {
		return nil, errors.New("invalid jpeg")
	}
	var out bytes.Buffer
	out.WriteByte(markerStart)
	out.WriteByte(markerSOI)
	for _, s := range segs {
		writeAppSegment(&out, s.marker, s.payload)
	}
	out.Write(jpegData[2:])
	return out.Bytes(), nil
}
