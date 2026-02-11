package ultrahdr

import (
	"bytes"
	"encoding/binary"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestSplitJoinRoundTripWithSampleJPEG(t *testing.T) {
	var (
		result *ResizeResult
		split  *SplitResult
	)

	// Use a known valid UltraHDR JPEG.
	err := ResizeUltraHDRFile("testdata/uhdr.jpg", "testdata/uhdr_thumb.jpg", 2400, 1600,
		func(opts *ResizeOptions) {
			opts.OnResult = func(res *ResizeResult) {
				result = res
			}
			opts.OnSplit = func(sr *SplitResult) {
				split = sr
			}
		})
	if err != nil {
		t.Fatalf("resize uhdr: %v", err)
	}

	if result == nil {
		t.Fatalf("resize result missing")
	}

	if split == nil {
		t.Fatalf("split result missing")
	}

	if split.Meta == nil {
		t.Fatalf("metadata missing")
	}

	// Repack without re-encoding to validate container assembly only.
	repacked, err := split.Join()
	if err != nil {
		t.Fatalf("repack join: %v", err)
	}
	if err := os.WriteFile(filepath.FromSlash("testdata/uhdr_repacked.jpg"), repacked, 0o644); err != nil {
		t.Fatalf("write uhdr_repacked.jpg: %v", err)
	}

	sr2, err := Split(result.Container)
	if err != nil {
		t.Fatalf("split after join: %v", err)
	}
	p2 := sr2.PrimaryJPEG
	g2 := sr2.GainmapJPEG
	meta2 := sr2.Meta

	if len(p2) < 4 || p2[0] != 0xFF || p2[1] != 0xD8 || p2[len(p2)-2] != 0xFF || p2[len(p2)-1] != 0xD9 {
		t.Fatalf("primary jpeg invalid markers")
	}
	if len(g2) < 4 || g2[0] != 0xFF || g2[1] != 0xD8 || g2[len(g2)-2] != 0xFF || g2[len(g2)-1] != 0xD9 {
		t.Fatalf("gainmap jpeg invalid markers")
	}
	if meta2 == nil || meta2.Version == "" {
		t.Fatalf("metadata missing")
	}
	// Compare marker sequence and MPF offsets against vips output.
	vipsData, err := os.ReadFile(filepath.FromSlash("testdata/uh-th.jpg"))
	if err != nil {
		t.Fatalf("read uh-th.jpg: %v", err)
	}
	seqWant, err := markerSequence(vipsData)
	if err != nil {
		t.Fatalf("marker sequence vips: %v", err)
	}
	seqGot, err := markerSequence(result.Container)
	if err != nil {
		t.Fatalf("marker sequence got: %v", err)
	}
	if seqWant != seqGot {
		t.Fatalf("marker sequence mismatch\nwant: %q\ngot:  %q", seqWant, seqGot)
	}
	wantMpf, err := parseMpfEntries(vipsData)
	if err != nil {
		t.Fatalf("parse mpf vips: %v", err)
	}
	gotMpf, err := parseMpfEntries(result.Container)
	if err != nil {
		t.Fatalf("parse mpf got: %v", err)
	}
	if err := validateMpfEntries(vipsData, wantMpf); err != nil {
		t.Fatalf("mpf vips invalid: %v", err)
	}
	if err := validateMpfEntries(result.Container, gotMpf); err != nil {
		t.Fatalf("mpf output invalid: %v", err)
	}
}

type mpfEntries struct {
	PrimarySize     uint32
	PrimaryOffset   uint32
	SecondarySize   uint32
	SecondaryOffset uint32
}

func markerSequence(data []byte) (string, error) {
	if len(data) < 2 || data[0] != 0xFF || data[1] != 0xD8 {
		return "", errors.New("jpeg missing SOI")
	}
	i := 2
	var out []byte
	for i < len(data) {
		if data[i] != 0xFF {
			j := bytes.Index(data[i:], []byte{0xFF, 0xD9})
			if j < 0 {
				return "", errors.New("jpeg missing EOI")
			}
			i += j
		}
		for i < len(data) && data[i] == 0xFF {
			i++
		}
		if i >= len(data) {
			break
		}
		marker := data[i]
		i++
		if marker == 0xD9 {
			out = append(out, 'E', 'O', 'I', ';')
			break
		}
		if marker == 0xDA {
			if i+2 > len(data) {
				return "", errors.New("jpeg truncated SOS")
			}
			ln := int(binary.BigEndian.Uint16(data[i : i+2]))
			out = append(out, 'S', 'O', 'S', ';')
			i += ln
			continue
		}
		if marker >= 0xD0 && marker <= 0xD7 {
			out = append(out, 'R', 'S', 'T', ';')
			continue
		}
		if i+2 > len(data) {
			return "", errors.New("jpeg truncated segment")
		}
		ln := int(binary.BigEndian.Uint16(data[i : i+2]))
		if ln < 2 || i+ln > len(data) {
			return "", errors.New("jpeg invalid segment length")
		}
		payload := data[i+2 : i+ln]
		label := markerLabel(marker, payload)
		out = append(out, label...)
		out = append(out, ';')
		i += ln
	}
	return string(out), nil
}

func markerLabel(marker byte, payload []byte) []byte {
	switch marker {
	case 0xE1:
		if bytes.HasPrefix(payload, []byte("Exif\x00\x00")) {
			return []byte("APP1:EXIF")
		}
		if bytes.HasPrefix(payload, append([]byte(xmpNamespace), 0)) {
			return []byte("APP1:XMP")
		}
		return []byte("APP1")
	case 0xE2:
		if bytes.HasPrefix(payload, mpfSig) {
			return []byte("APP2:MPF")
		}
		if bytes.HasPrefix(payload, []byte("ICC_PROFILE")) {
			return []byte("APP2:ICC")
		}
		if bytes.HasPrefix(payload, append([]byte(isoNamespace), 0)) {
			return []byte("APP2:ISO")
		}
		return []byte("APP2")
	case 0xDB:
		return []byte("DQT")
	case 0xC4:
		return []byte("DHT")
	case 0xC0:
		return []byte("SOF0")
	default:
		return []byte("M")
	}
}

func parseMpfEntries(data []byte) (mpfEntries, error) {
	_, payload, err := findMpfPayload(data)
	if err != nil {
		return mpfEntries{}, err
	}
	if len(payload) < len(mpfSig)+mpfEndianSize+4+2 {
		return mpfEntries{}, errors.New("mpf payload too small")
	}
	if !bytes.HasPrefix(payload, mpfSig) {
		return mpfEntries{}, errors.New("mpf signature missing")
	}
	if !bytes.Equal(payload[len(mpfSig):len(mpfSig)+4], mpfBigEndian) {
		return mpfEntries{}, errors.New("mpf endian mismatch")
	}
	off := len(mpfSig) + 4
	ifdOffset := int(binary.BigEndian.Uint32(payload[off : off+4]))
	if ifdOffset < 0 || ifdOffset+2 > len(payload) {
		return mpfEntries{}, errors.New("mpf ifd offset invalid")
	}
	ifd := payload[len(mpfSig):]
	if ifdOffset+2 > len(ifd) {
		return mpfEntries{}, errors.New("mpf ifd truncated")
	}
	count := int(binary.BigEndian.Uint16(ifd[ifdOffset : ifdOffset+2]))
	pos := ifdOffset + 2
	var entryOffset int
	for i := 0; i < count; i++ {
		if pos+12 > len(ifd) {
			return mpfEntries{}, errors.New("mpf entry truncated")
		}
		tag := binary.BigEndian.Uint16(ifd[pos : pos+2])
		typ := binary.BigEndian.Uint16(ifd[pos+2 : pos+4])
		_ = typ
		countVal := binary.BigEndian.Uint32(ifd[pos+4 : pos+8])
		value := binary.BigEndian.Uint32(ifd[pos+8 : pos+12])
		if tag == mpfEntryTag && countVal == mpfEntrySize*mpfNumPictures {
			entryOffset = int(value)
			break
		}
		pos += 12
	}
	if entryOffset == 0 {
		return mpfEntries{}, errors.New("mpf entries not found")
	}
	if entryOffset+mpfEntrySize*mpfNumPictures > len(ifd) {
		return mpfEntries{}, errors.New("mpf entry data truncated")
	}
	entries := ifd[entryOffset : entryOffset+mpfEntrySize*mpfNumPictures]

	parse := func(b []byte) (size, offset uint32) {
		size = binary.BigEndian.Uint32(b[4:8])
		offset = binary.BigEndian.Uint32(b[8:12])
		return
	}

	pSize, pOff := parse(entries[:mpfEntrySize])
	sSize, sOff := parse(entries[mpfEntrySize:])
	return mpfEntries{
		PrimarySize:     pSize,
		PrimaryOffset:   pOff,
		SecondarySize:   sSize,
		SecondaryOffset: sOff,
	}, nil
}

func validateMpfEntries(data []byte, entries mpfEntries) error {
	mpfStart, _, err := findMpfPayload(data)
	if err != nil {
		return err
	}
	ranges, err := scanJPEGs(data)
	if err != nil || len(ranges) < 2 {
		return errors.New("jpeg ranges not found")
	}
	primarySize := uint32(ranges[0][1] - ranges[0][0])
	secondarySize := uint32(ranges[1][1] - ranges[1][0])
	secondaryOffset := uint32(ranges[1][0] - (mpfStart + 4))
	if entries.PrimaryOffset != 0 {
		return errors.New("primary offset is not zero")
	}
	if entries.PrimarySize != primarySize {
		return errors.New("primary size mismatch")
	}
	if entries.SecondarySize != secondarySize {
		return errors.New("secondary size mismatch")
	}
	if entries.SecondaryOffset != secondaryOffset {
		return errors.New("secondary offset mismatch")
	}
	return nil
}

func findMpfPayload(data []byte) (int, []byte, error) {
	if len(data) < 2 || data[0] != 0xFF || data[1] != 0xD8 {
		return 0, nil, errors.New("jpeg missing SOI")
	}
	i := 2
	for i < len(data) {
		if data[i] != 0xFF {
			j := bytes.Index(data[i:], []byte{0xFF, 0xD9})
			if j < 0 {
				return 0, nil, errors.New("jpeg missing EOI")
			}
			i += j
		}
		for i < len(data) && data[i] == 0xFF {
			i++
		}
		if i >= len(data) {
			break
		}
		marker := data[i]
		i++
		if marker == 0xD9 || marker == 0xDA {
			break
		}
		if marker >= 0xD0 && marker <= 0xD7 {
			continue
		}
		if i+2 > len(data) {
			return 0, nil, errors.New("jpeg truncated segment")
		}
		ln := int(binary.BigEndian.Uint16(data[i : i+2]))
		if ln < 2 || i+ln > len(data) {
			return 0, nil, errors.New("jpeg invalid segment length")
		}
		payload := data[i+2 : i+ln]
		if marker == 0xE2 && bytes.HasPrefix(payload, mpfSig) {
			return i + 2, payload, nil
		}
		i += ln
	}
	return 0, nil, errors.New("mpf segment not found")
}
