package ultrahdr

import (
	"bytes"
	"compress/zlib"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math"
	"strings"
)

const exrMagic = 20000630

const (
	exrCompressionNone = 0
	exrCompressionZips = 2
	exrCompressionZip  = 3
)

const (
	exrPixelUint  = 0
	exrPixelHalf  = 1
	exrPixelFloat = 2
)

const (
	exrChanOther = -2
	exrChanY     = -1
	exrChanR     = 0
	exrChanG     = 1
	exrChanB     = 2
)

type HDRImage struct {
	W, H int
	Pix  []float32
}

func (h *HDRImage) At(x, y int) rgb {
	if x < 0 {
		x = 0
	}
	if y < 0 {
		y = 0
	}
	if x >= h.W {
		x = h.W - 1
	}
	if y >= h.H {
		y = h.H - 1
	}
	i := (y*h.W + x) * 3
	return rgb{r: h.Pix[i], g: h.Pix[i+1], b: h.Pix[i+2]}
}

type exrChannel struct {
	name      string
	pixelType int32
	xSampling int32
	ySampling int32
	role      int
}

func DecodeEXR(data []byte) (*HDRImage, error) {
	r := bytes.NewReader(data)
	magic, err := readU32(r)
	if err != nil {
		return nil, err
	}
	if magic != exrMagic {
		return nil, errors.New("not an OpenEXR file")
	}
	version, err := readU32(r)
	if err != nil {
		return nil, err
	}
	if version&0x00000200 != 0 {
		return nil, errors.New("tiled OpenEXR not supported")
	}
	if version&0x00000800 != 0 {
		return nil, errors.New("multipart OpenEXR not supported")
	}
	if version&0x00000400 != 0 {
		return nil, errors.New("deep OpenEXR not supported")
	}

	var channels []exrChannel
	var dataWindow [4]int32
	var hasDataWindow bool
	var compression byte = exrCompressionNone

	for {
		name, err := readNullString(r)
		if err != nil {
			return nil, err
		}
		if name == "" {
			break
		}
		typ, err := readNullString(r)
		if err != nil {
			return nil, err
		}
		size, err := readI32(r)
		if err != nil {
			return nil, err
		}
		if size < 0 {
			return nil, errors.New("invalid EXR attribute size")
		}
		payload := make([]byte, size)
		if _, err := io.ReadFull(r, payload); err != nil {
			return nil, err
		}

		switch name {
		case "channels":
			if typ != "chlist" {
				return nil, errors.New("unexpected channels attribute type")
			}
			ch, err := parseEXRChannels(payload)
			if err != nil {
				return nil, err
			}
			channels = ch
		case "dataWindow":
			if typ != "box2i" {
				return nil, errors.New("unexpected dataWindow attribute type")
			}
			if len(payload) != 16 {
				return nil, errors.New("invalid dataWindow payload")
			}
			dataWindow[0] = int32(binary.LittleEndian.Uint32(payload[0:4]))
			dataWindow[1] = int32(binary.LittleEndian.Uint32(payload[4:8]))
			dataWindow[2] = int32(binary.LittleEndian.Uint32(payload[8:12]))
			dataWindow[3] = int32(binary.LittleEndian.Uint32(payload[12:16]))
			hasDataWindow = true
		case "compression":
			if typ != "compression" || len(payload) < 1 {
				return nil, errors.New("invalid compression attribute")
			}
			compression = payload[0]
		case "tiles":
			return nil, errors.New("tiled OpenEXR not supported")
		}
	}

	if len(channels) == 0 {
		return nil, errors.New("OpenEXR missing channels")
	}
	if !hasDataWindow {
		return nil, errors.New("OpenEXR missing dataWindow")
	}
	for _, ch := range channels {
		if ch.xSampling != 1 || ch.ySampling != 1 {
			return nil, errors.New("OpenEXR subsampled channels are not supported")
		}
	}
	if compression != exrCompressionNone && compression != exrCompressionZips && compression != exrCompressionZip {
		return nil, fmt.Errorf("unsupported OpenEXR compression %d", compression)
	}

	width := int(dataWindow[2]-dataWindow[0]) + 1
	height := int(dataWindow[3]-dataWindow[1]) + 1
	if width <= 0 || height <= 0 {
		return nil, errors.New("invalid OpenEXR dimensions")
	}

	blockLines := 1
	if compression == exrCompressionZip {
		blockLines = 16
	}
	blockCount := (height + blockLines - 1) / blockLines
	offsets := make([]uint64, blockCount)
	for i := range offsets {
		v, err := readU64(r)
		if err != nil {
			return nil, err
		}
		offsets[i] = v
	}

	hdr := &HDRImage{
		W:   width,
		H:   height,
		Pix: make([]float32, width*height*3),
	}

	baseY := int(dataWindow[1])
	for block := 0; block < blockCount; block++ {
		if offsets[block] == 0 {
			continue
		}
		if _, err := r.Seek(int64(offsets[block]), io.SeekStart); err != nil {
			return nil, err
		}
		y, err := readI32(r)
		if err != nil {
			return nil, err
		}
		dataSize, err := readI32(r)
		if err != nil {
			return nil, err
		}
		if dataSize < 0 {
			return nil, errors.New("invalid OpenEXR block size")
		}
		raw := make([]byte, dataSize)
		if _, err := io.ReadFull(r, raw); err != nil {
			return nil, err
		}

		startY := int(y) - baseY
		if startY < 0 || startY >= height {
			return nil, errors.New("OpenEXR scanline out of bounds")
		}
		lines := blockLines
		if startY+lines > height {
			lines = height - startY
		}

		expected := exrExpectedBlockBytes(width, lines, channels)
		unpacked, err := exrDecompress(compression, raw, expected)
		if err != nil {
			return nil, err
		}

		if err := exrDecodeBlock(hdr, channels, startY, width, lines, unpacked); err != nil {
			return nil, err
		}
	}

	if !hasRGBOrY(channels) {
		return nil, errors.New("OpenEXR missing R/G/B or Y channels")
	}
	return hdr, nil
}

func parseEXRChannels(data []byte) ([]exrChannel, error) {
	r := bytes.NewReader(data)
	var channels []exrChannel
	for {
		name, err := readNullString(r)
		if err != nil {
			return nil, err
		}
		if name == "" {
			break
		}
		pixelType, err := readI32(r)
		if err != nil {
			return nil, err
		}
		if pixelType != exrPixelHalf && pixelType != exrPixelFloat && pixelType != exrPixelUint {
			return nil, fmt.Errorf("unsupported OpenEXR pixel type %d", pixelType)
		}
		if _, err := r.ReadByte(); err != nil {
			return nil, err
		}
		if _, err := r.Seek(3, io.SeekCurrent); err != nil {
			return nil, err
		}
		xSampling, err := readI32(r)
		if err != nil {
			return nil, err
		}
		ySampling, err := readI32(r)
		if err != nil {
			return nil, err
		}
		role := exrChanOther
		switch strings.ToUpper(name) {
		case "R":
			role = exrChanR
		case "G":
			role = exrChanG
		case "B":
			role = exrChanB
		case "Y":
			role = exrChanY
		}
		channels = append(channels, exrChannel{
			name:      name,
			pixelType: pixelType,
			xSampling: xSampling,
			ySampling: ySampling,
			role:      role,
		})
	}
	return channels, nil
}

func exrExpectedBlockBytes(width, lines int, channels []exrChannel) int {
	total := 0
	for _, ch := range channels {
		bpp := 0
		switch ch.pixelType {
		case exrPixelHalf:
			bpp = 2
		case exrPixelFloat, exrPixelUint:
			bpp = 4
		}
		total += width * lines * bpp
	}
	return total
}

func exrDecompress(compression byte, data []byte, expected int) ([]byte, error) {
	switch compression {
	case exrCompressionNone:
		if expected > 0 && len(data) != expected {
			return nil, errors.New("unexpected OpenEXR block size")
		}
		return data, nil
	case exrCompressionZips, exrCompressionZip:
		zr, err := zlib.NewReader(bytes.NewReader(data))
		if err != nil {
			return nil, err
		}
		defer zr.Close()
		uncompressed, err := io.ReadAll(zr)
		if err != nil {
			return nil, err
		}
		if expected > 0 && len(uncompressed) != expected {
			return nil, errors.New("unexpected OpenEXR decompressed size")
		}
		if len(uncompressed)%2 != 0 {
			return nil, errors.New("invalid OpenEXR ZIP payload size")
		}
		undoPredictor(uncompressed)
		return unshuffleBytes(uncompressed), nil
	default:
		return nil, errors.New("unsupported OpenEXR compression")
	}
}

func undoPredictor(data []byte) {
	for i := 1; i < len(data); i++ {
		data[i] = byte(int(data[i]) + int(data[i-1]) - 128)
	}
}

func unshuffleBytes(data []byte) []byte {
	n := len(data) / 2
	out := make([]byte, len(data))
	for i := 0; i < n; i++ {
		out[2*i] = data[i]
		out[2*i+1] = data[i+n]
	}
	return out
}

func exrDecodeBlock(dst *HDRImage, channels []exrChannel, startY, width, lines int, data []byte) error {
	offset := 0
	for row := 0; row < lines; row++ {
		y := startY + row
		for _, ch := range channels {
			bpp := 0
			switch ch.pixelType {
			case exrPixelHalf:
				bpp = 2
			case exrPixelFloat, exrPixelUint:
				bpp = 4
			default:
				return errors.New("unsupported OpenEXR channel pixel type")
			}
			lineBytes := width * bpp
			if offset+lineBytes > len(data) {
				return errors.New("OpenEXR block truncated")
			}
			line := data[offset : offset+lineBytes]
			offset += lineBytes

			switch ch.role {
			case exrChanR, exrChanG, exrChanB, exrChanY:
				if err := exrApplyLine(dst, ch.role, y, width, ch.pixelType, line); err != nil {
					return err
				}
			default:
				continue
			}
		}
	}
	return nil
}

func exrApplyLine(dst *HDRImage, role int, y, width int, pixelType int32, line []byte) error {
	for x := 0; x < width; x++ {
		var v float32
		switch pixelType {
		case exrPixelHalf:
			off := x * 2
			v = halfToFloat32(binary.LittleEndian.Uint16(line[off : off+2]))
		case exrPixelFloat:
			off := x * 4
			v = math.Float32frombits(binary.LittleEndian.Uint32(line[off : off+4]))
		case exrPixelUint:
			off := x * 4
			v = float32(binary.LittleEndian.Uint32(line[off : off+4]))
		default:
			return errors.New("unsupported OpenEXR pixel type")
		}
		idx := (y*dst.W + x) * 3
		switch role {
		case exrChanR:
			dst.Pix[idx] = v
		case exrChanG:
			dst.Pix[idx+1] = v
		case exrChanB:
			dst.Pix[idx+2] = v
		case exrChanY:
			dst.Pix[idx] = v
			dst.Pix[idx+1] = v
			dst.Pix[idx+2] = v
		}
	}
	return nil
}

func hasRGBOrY(channels []exrChannel) bool {
	for _, ch := range channels {
		if ch.role == exrChanR || ch.role == exrChanG || ch.role == exrChanB || ch.role == exrChanY {
			return true
		}
	}
	return false
}

func readNullString(r *bytes.Reader) (string, error) {
	var buf []byte
	for {
		b, err := r.ReadByte()
		if err != nil {
			return "", err
		}
		if b == 0 {
			break
		}
		buf = append(buf, b)
	}
	return string(buf), nil
}

func readU32(r *bytes.Reader) (uint32, error) {
	var buf [4]byte
	if _, err := io.ReadFull(r, buf[:]); err != nil {
		return 0, err
	}
	return binary.LittleEndian.Uint32(buf[:]), nil
}

func readU64(r *bytes.Reader) (uint64, error) {
	var buf [8]byte
	if _, err := io.ReadFull(r, buf[:]); err != nil {
		return 0, err
	}
	return binary.LittleEndian.Uint64(buf[:]), nil
}

func readI32(r *bytes.Reader) (int32, error) {
	v, err := readU32(r)
	return int32(v), err
}

func halfToFloat32(h uint16) float32 {
	sign := uint32(h>>15) & 0x1
	exp := int32(h>>10) & 0x1F
	mant := int32(h & 0x03FF)

	if exp == 0 {
		if mant == 0 {
			return math.Float32frombits(sign << 31)
		}
		for mant&0x0400 == 0 {
			mant <<= 1
			exp--
		}
		exp++
		mant &= 0x03FF
	} else if exp == 31 {
		if mant == 0 {
			return math.Float32frombits((sign << 31) | 0x7F800000)
		}
		return math.Float32frombits((sign << 31) | 0x7F800000 | (uint32(mant) << 13))
	}

	exp = exp + (127 - 15)
	mant <<= 13
	bits := (sign << 31) | (uint32(exp) << 23) | uint32(mant)
	return math.Float32frombits(bits)
}
