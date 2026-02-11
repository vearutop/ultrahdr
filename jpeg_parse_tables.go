package ultrahdr

import (
	"encoding/binary"
	"errors"
	"fmt"

	"github.com/vearutop/ultrahdr/internal/jpegx"
)

type jpegTables struct {
	Quant    [2][64]byte
	Huff     [4]jpegx.HuffmanSpec
	Sampling [3]jpegx.SamplingFactor
	HasQuant bool
	HasHuff  bool
	HasSOF0  bool
}

func extractJpegTables(data []byte) (*jpegTables, error) {
	if len(data) < 4 || data[0] != 0xFF || data[1] != 0xD8 {
		return nil, errors.New("invalid jpeg")
	}
	t := &jpegTables{}
	pos := 2
	for pos+3 < len(data) {
		if data[pos] != 0xFF {
			pos++
			continue
		}
		for pos < len(data) && data[pos] == 0xFF {
			pos++
		}
		if pos >= len(data) {
			break
		}
		marker := data[pos]
		pos++
		if marker == 0xDA || marker == 0xD9 {
			break
		}
		if marker >= 0xD0 && marker <= 0xD7 {
			continue
		}
		if pos+1 >= len(data) {
			return nil, errors.New("truncated marker")
		}
		segLen := int(binary.BigEndian.Uint16(data[pos:]))
		if segLen < 2 || pos+segLen > len(data) {
			return nil, errors.New("invalid segment length")
		}
		seg := data[pos+2 : pos+segLen]
		switch marker {
		case 0xDB: // DQT
			if err := parseDQT(seg, t); err != nil {
				return nil, err
			}
		case 0xC4: // DHT
			if err := parseDHT(seg, t); err != nil {
				return nil, err
			}
		case 0xC0: // SOF0
			if err := parseSOF0(seg, t); err != nil {
				return nil, err
			}
		}
		pos += segLen
	}
	if !t.HasQuant || !t.HasHuff || !t.HasSOF0 {
		return nil, errors.New("missing tables or SOF0")
	}
	return t, nil
}

func parseDQT(seg []byte, t *jpegTables) error {
	pos := 0
	for pos < len(seg) {
		if pos+1 > len(seg) {
			return errors.New("truncated dqt")
		}
		pq := seg[pos] >> 4
		tq := seg[pos] & 0x0F
		pos++
		if pq != 0 {
			return errors.New("unsupported 16-bit quant table")
		}
		if pos+64 > len(seg) {
			return errors.New("truncated dqt table")
		}
		if tq > 1 {
			pos += 64
			continue
		}
		copy(t.Quant[tq][:], seg[pos:pos+64])
		t.HasQuant = true
		pos += 64
	}
	return nil
}

func parseDHT(seg []byte, t *jpegTables) error {
	pos := 0
	for pos < len(seg) {
		if pos+17 > len(seg) {
			return errors.New("truncated dht")
		}
		tc := seg[pos] >> 4
		th := seg[pos] & 0x0F
		pos++
		var count [16]byte
		copy(count[:], seg[pos:pos+16])
		pos += 16
		total := 0
		for _, c := range count {
			total += int(c)
		}
		if pos+total > len(seg) {
			return errors.New("truncated dht values")
		}
		vals := append([]byte(nil), seg[pos:pos+total]...)
		pos += total

		idx := -1
		switch {
		case tc == 0 && th == 0:
			idx = 0
		case tc == 1 && th == 0:
			idx = 1
		case tc == 0 && th == 1:
			idx = 2
		case tc == 1 && th == 1:
			idx = 3
		}
		if idx >= 0 {
			t.Huff[idx] = jpegx.HuffmanSpec{Count: count, Value: vals}
			t.HasHuff = true
		}
	}
	return nil
}

func parseSOF0(seg []byte, t *jpegTables) error {
	if len(seg) < 6 {
		return errors.New("truncated sof0")
	}
	precision := seg[0]
	if precision != 8 {
		return fmt.Errorf("unsupported precision %d", precision)
	}
	n := int(seg[5])
	if n < 1 {
		return errors.New("invalid component count")
	}
	pos := 6
	for i := 0; i < n && i < 3; i++ {
		if pos+3 > len(seg) {
			return errors.New("truncated sof0 comps")
		}
		// cid := seg[pos]
		samp := seg[pos+1]
		h := samp >> 4
		v := samp & 0x0F
		t.Sampling[i] = jpegx.SamplingFactor{H: h, V: v}
		pos += 3
	}
	// If only one component, set to 1x1
	if n == 1 {
		t.Sampling[0] = jpegx.SamplingFactor{H: 1, V: 1}
	}
	t.HasSOF0 = true
	return nil
}
