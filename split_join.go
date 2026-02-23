package ultrahdr

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"errors"
	"io"
)

// Result contains the primary/gainmap JPEGs with optional container and metadata.
type Result struct {
	Container []byte
	Primary   []byte
	Gainmap   []byte
	Meta      *GainMapMetadata
	Segs      *MetadataSegments
}

// Split extracts primary/gainmap JPEGs, metadata, and raw XMP/ISO segments.
func Split(r io.Reader) (*Result, error) {
	if r == nil {
		return nil, errors.New("missing reader")
	}

	br := bufio.NewReader(r)
	res := Result{Segs: &MetadataSegments{}}

	var (
		primaryApp1 [][]byte
		primaryApp2 [][]byte
		gainmapApp1 [][]byte
		gainmapApp2 [][]byte
	)

	if err := scanToSOI(br, &res.Primary); err != nil {
		return nil, err
	}
	if err := readJPEGFromSOI(br, &res.Primary, &primaryApp1, &primaryApp2, true); err != nil {
		return nil, err
	}
	if err := scanToSOI(br, &res.Gainmap); err != nil {
		return nil, errors.New("gainmap image not found")
	}
	if err := readJPEGFromSOI(br, &res.Gainmap, &gainmapApp1, &gainmapApp2, false); err != nil {
		return nil, err
	}

	res.Segs.PrimaryXMP = findXMP(primaryApp1)
	res.Segs.PrimaryISO = findISO(primaryApp2)
	res.Segs.SecondaryXMP = findXMP(gainmapApp1)
	res.Segs.SecondaryISO = findISO(gainmapApp2)

	var err error
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
func (sr Result) Join() ([]byte, error) {
	if sr.Segs == nil {
		return nil, errors.New("segments required")
	}
	return assembleContainerWithSegments(sr.Primary, sr.Gainmap, sr.Segs)
}

func scanToSOI(br *bufio.Reader, dst *[]byte) error {
	var (
		prevWasFF bool
		buf       bytes.Buffer
	)
	for {
		b, err := br.ReadByte()
		if err != nil {
			return err
		}
		if prevWasFF && b == markerSOI {
			buf.WriteByte(markerStart)
			buf.WriteByte(markerSOI)
			*dst = buf.Bytes()
			return nil
		}
		prevWasFF = b == markerStart
	}
}

func readJPEGFromSOI(br *bufio.Reader, dst *[]byte, app1, app2 *[][]byte, stopOnMPF bool) error {
	var (
		buf         bytes.Buffer
		stopCapture bool
	)
	buf.Write(*dst)
	for {
		marker, err := readMarkerWithCopy(br, &buf)
		if err != nil {
			return err
		}
		switch {
		case marker == markerEOI:
			*dst = buf.Bytes()
			return nil
		case marker == markerSOS:
			if err := readSegment(br, &buf, nil); err != nil {
				return err
			}
			if err := readScanData(br, &buf); err != nil {
				return err
			}
			*dst = buf.Bytes()
			return nil
		case marker >= 0xD0 && marker <= 0xD7:
			continue
		case marker == markerSOI:
			continue
		default:
			var payload []byte
			if err := readSegment(br, &buf, &payload); err != nil {
				return err
			}
			if stopCapture {
				continue
			}
			switch marker {
			case markerAPP1:
				*app1 = append(*app1, append([]byte(nil), payload...))
			case markerAPP2:
				*app2 = append(*app2, append([]byte(nil), payload...))
				if stopOnMPF && bytes.HasPrefix(payload, mpfSig) {
					stopCapture = true
				}
			}
		}
	}
}

func readMarkerWithCopy(br *bufio.Reader, buf *bytes.Buffer) (byte, error) {
	for {
		b, err := br.ReadByte()
		if err != nil {
			return 0, err
		}
		buf.WriteByte(b)
		if b != markerStart {
			continue
		}
		for {
			b2, err := br.ReadByte()
			if err != nil {
				return 0, err
			}
			buf.WriteByte(b2)
			if b2 == markerStart {
				continue
			}
			return b2, nil
		}
	}
}

func readSegment(br *bufio.Reader, buf *bytes.Buffer, payload *[]byte) error {
	var lenBytes [2]byte
	if _, err := io.ReadFull(br, lenBytes[:]); err != nil {
		return err
	}
	buf.Write(lenBytes[:])
	segLen := int(binary.BigEndian.Uint16(lenBytes[:]))
	if segLen < 2 {
		return errors.New("invalid segment length")
	}
	payloadLen := segLen - 2
	if payloadLen == 0 {
		if payload != nil {
			*payload = nil
		}
		return nil
	}
	segment := make([]byte, payloadLen)
	if _, err := io.ReadFull(br, segment); err != nil {
		return err
	}
	buf.Write(segment)
	if payload != nil {
		*payload = segment
	}
	return nil
}

func readScanData(br *bufio.Reader, buf *bytes.Buffer) error {
	for {
		b, err := br.ReadByte()
		if err != nil {
			return err
		}
		buf.WriteByte(b)
		if b != markerStart {
			continue
		}
		b2, err := br.ReadByte()
		if err != nil {
			return err
		}
		buf.WriteByte(b2)
		switch {
		case b2 == 0x00:
			continue
		case b2 >= 0xD0 && b2 <= 0xD7:
			continue
		case b2 == markerEOI:
			return nil
		default:
			if err := readSegment(br, buf, nil); err != nil {
				return err
			}
		}
	}
}
