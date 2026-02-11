package ultrahdr

import (
	"bufio"
	"bytes"
	"errors"
	"io"
)

var (
	xmpPrefix = append([]byte(xmpNamespace), 0)
	isoPrefix = append([]byte(isoNamespace), 0)
)

// IsUltraHDR performs a streaming UltraHDR check without loading the full image.
// It reads until the gainmap header is reached and scans APP metadata for XMP/ISO.
func IsUltraHDR(r io.Reader) (bool, error) {
	br := bufio.NewReader(r)
	found, err := findSOI(br)
	if err != nil {
		return false, err
	}
	if !found {
		return false, nil
	}
	if err := skipJPEG(br); err != nil {
		return false, err
	}
	found, err = findSOI(br)
	if err != nil {
		return false, err
	}
	if !found {
		return false, nil
	}
	return checkGainmapHeader(br)
}

func findSOI(br *bufio.Reader) (bool, error) {
	var prev byte
	for {
		b, err := br.ReadByte()
		if err != nil {
			if errors.Is(err, io.EOF) {
				return false, nil
			}
			return false, err
		}
		if prev == markerStart && b == markerSOI {
			return true, nil
		}
		prev = b
	}
}

func skipJPEG(br *bufio.Reader) error {
	for {
		marker, err := readMarker(br)
		if err != nil {
			return err
		}
		switch marker {
		case markerEOI:
			return nil
		case markerSOS:
			return skipScanToEOI(br)
		default:
			if err := discardSegment(br); err != nil {
				return err
			}
		}
	}
}

func checkGainmapHeader(br *bufio.Reader) (bool, error) {
	for {
		marker, err := readMarker(br)
		if err != nil {
			return false, err
		}
		switch marker {
		case markerEOI:
			return false, nil
		case markerSOS:
			return false, nil
		case markerAPP1, markerAPP2:
			match, err := segmentHasGainmapMetadata(br, marker)
			if err != nil {
				return false, err
			}
			if match {
				return true, nil
			}
		default:
			if err := discardSegment(br); err != nil {
				return false, err
			}
		}
	}
}

func readMarker(br *bufio.Reader) (byte, error) {
	for {
		b, err := br.ReadByte()
		if err != nil {
			return 0, err
		}
		if b != markerStart {
			continue
		}
		for {
			m, err := br.ReadByte()
			if err != nil {
				return 0, err
			}
			if m != markerStart {
				return m, nil
			}
		}
	}
}

func discardSegment(br *bufio.Reader) error {
	length, err := readU16(br)
	if err != nil {
		return err
	}
	if length < 2 {
		return errors.New("invalid segment length")
	}
	return discardN(br, int(length-2))
}

func segmentHasGainmapMetadata(br *bufio.Reader, marker byte) (bool, error) {
	length, err := readU16(br)
	if err != nil {
		return false, err
	}
	if length < 2 {
		return false, errors.New("invalid segment length")
	}
	payloadLen := int(length - 2)
	var prefix []byte
	if marker == markerAPP1 {
		prefix = xmpPrefix
	} else {
		prefix = isoPrefix
	}
	maxPrefix := len(prefix)
	readLen := payloadLen
	if readLen > maxPrefix {
		readLen = maxPrefix
	}
	buf := make([]byte, readLen)
	if _, err := io.ReadFull(br, buf); err != nil {
		return false, err
	}
	match := bytes.HasPrefix(buf, prefix)
	if payloadLen > readLen {
		if err := discardN(br, payloadLen-readLen); err != nil {
			return false, err
		}
	}
	return match, nil
}

func readU16(br *bufio.Reader) (uint16, error) {
	hi, err := br.ReadByte()
	if err != nil {
		return 0, err
	}
	lo, err := br.ReadByte()
	if err != nil {
		return 0, err
	}
	return uint16(hi)<<8 | uint16(lo), nil
}

func discardN(br *bufio.Reader, n int) error {
	if n <= 0 {
		return nil
	}
	_, err := io.CopyN(io.Discard, br, int64(n))
	return err
}

func skipScanToEOI(br *bufio.Reader) error {
	for {
		b, err := br.ReadByte()
		if err != nil {
			return err
		}
		if b != markerStart {
			continue
		}
		m, err := br.ReadByte()
		if err != nil {
			return err
		}
		for m == markerStart {
			m, err = br.ReadByte()
			if err != nil {
				return err
			}
		}
		if m == 0x00 {
			continue
		}
		if m >= 0xD0 && m <= 0xD7 {
			continue
		}
		if m == markerEOI {
			return nil
		}
	}
}
