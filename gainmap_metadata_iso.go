package ultrahdr

import (
	"encoding/binary"
	"errors"
	"math"
)

const (
	isoIsMultiChannelMask = 1 << 7
	isoUseBaseColorMask   = 1 << 6
)

type gainmapMetadataFrac struct {
	GainMapMinN       [3]int32
	GainMapMinD       [3]uint32
	GainMapMaxN       [3]int32
	GainMapMaxD       [3]uint32
	GainMapGammaN     [3]uint32
	GainMapGammaD     [3]uint32
	BaseOffsetN       [3]int32
	BaseOffsetD       [3]uint32
	AltOffsetN        [3]int32
	AltOffsetD        [3]uint32
	BaseHdrHeadroomN  uint32
	BaseHdrHeadroomD  uint32
	AltHdrHeadroomN   uint32
	AltHdrHeadroomD   uint32
	BackwardDirection bool
	UseBaseColorSpace bool
}

func decodeGainmapMetadataISO(data []byte) (*GainMapMetadata, error) {
	var frac gainmapMetadataFrac
	if err := frac.decode(data); err != nil {
		return nil, err
	}
	meta := GainMapMetadata{Version: jpegrVersion}
	fracToFloat(&frac, &meta)

	return &meta, nil
}

func encodeGainmapMetadataISO(meta *GainMapMetadata) ([]byte, error) {
	if meta == nil {
		return nil, errors.New("gainmap metadata missing")
	}
	var frac gainmapMetadataFrac
	if err := gainmapMetadataFloatToFraction(meta, &frac); err != nil {
		return nil, err
	}
	return frac.encode()
}

func buildIsoPayload(meta *GainMapMetadata) ([]byte, error) {
	encoded, err := encodeGainmapMetadataISO(meta)
	if err != nil {
		return nil, err
	}
	payload := make([]byte, 0, len(isoNamespace)+1+len(encoded))
	payload = append(payload, []byte(isoNamespace)...)
	payload = append(payload, 0)
	payload = append(payload, encoded...)
	return payload, nil
}

func (m *gainmapMetadataFrac) decode(in []byte) error {
	pos := 0
	readU16 := func() (uint16, error) {
		if pos+2 > len(in) {
			return 0, errors.New("iso metadata truncated")
		}
		v := binary.BigEndian.Uint16(in[pos:])
		pos += 2
		return v, nil
	}
	readU32 := func() (uint32, error) {
		if pos+4 > len(in) {
			return 0, errors.New("iso metadata truncated")
		}
		v := binary.BigEndian.Uint32(in[pos:])
		pos += 4
		return v, nil
	}
	readS32 := func() (int32, error) {
		v, err := readU32()
		return int32(v), err
	}
	readU8 := func() (uint8, error) {
		if pos+1 > len(in) {
			return 0, errors.New("iso metadata truncated")
		}
		v := in[pos]
		pos++
		return v, nil
	}

	minVer, err := readU16()
	if err != nil {
		return err
	}
	if minVer != 0 {
		return errors.New("unsupported iso min_version")
	}
	if _, err = readU16(); err != nil {
		return err
	}

	flags, err := readU8()
	if err != nil {
		return err
	}
	channelCount := uint8(1)
	if (flags & isoIsMultiChannelMask) != 0 {
		channelCount = 3
	}
	if channelCount != 1 && channelCount != 3 {
		return errors.New("invalid channel count")
	}
	m.UseBaseColorSpace = (flags & isoUseBaseColorMask) != 0
	m.BackwardDirection = (flags & 4) != 0
	useCommon := (flags & 8) != 0

	if useCommon {
		common, err := readU32()
		if err != nil {
			return err
		}
		m.BaseHdrHeadroomD = common
		m.AltHdrHeadroomD = common
		m.BaseHdrHeadroomN, err = readU32()
		if err != nil {
			return err
		}
		m.AltHdrHeadroomN, err = readU32()
		if err != nil {
			return err
		}
		for c := 0; c < int(channelCount); c++ {
			if m.GainMapMinN[c], err = readS32(); err != nil {
				return err
			}
			m.GainMapMinD[c] = common
			if m.GainMapMaxN[c], err = readS32(); err != nil {
				return err
			}
			m.GainMapMaxD[c] = common
			if m.GainMapGammaN[c], err = readU32(); err != nil {
				return err
			}
			m.GainMapGammaD[c] = common
			if m.BaseOffsetN[c], err = readS32(); err != nil {
				return err
			}
			m.BaseOffsetD[c] = common
			if m.AltOffsetN[c], err = readS32(); err != nil {
				return err
			}
			m.AltOffsetD[c] = common
		}
		return nil
	}

	if m.BaseHdrHeadroomN, err = readU32(); err != nil {
		return err
	}
	if m.BaseHdrHeadroomD, err = readU32(); err != nil {
		return err
	}
	if m.AltHdrHeadroomN, err = readU32(); err != nil {
		return err
	}
	if m.AltHdrHeadroomD, err = readU32(); err != nil {
		return err
	}
	for c := 0; c < int(channelCount); c++ {
		if m.GainMapMinN[c], err = readS32(); err != nil {
			return err
		}
		if m.GainMapMinD[c], err = readU32(); err != nil {
			return err
		}
		if m.GainMapMaxN[c], err = readS32(); err != nil {
			return err
		}
		if m.GainMapMaxD[c], err = readU32(); err != nil {
			return err
		}
		if m.GainMapGammaN[c], err = readU32(); err != nil {
			return err
		}
		if m.GainMapGammaD[c], err = readU32(); err != nil {
			return err
		}
		if m.BaseOffsetN[c], err = readS32(); err != nil {
			return err
		}
		if m.BaseOffsetD[c], err = readU32(); err != nil {
			return err
		}
		if m.AltOffsetN[c], err = readS32(); err != nil {
			return err
		}
		if m.AltOffsetD[c], err = readU32(); err != nil {
			return err
		}
	}
	return nil
}

func (m *gainmapMetadataFrac) encode() ([]byte, error) {
	const minVersion uint16 = 0
	const writerVersion uint16 = 0

	channelCount := uint8(3)
	if m.allChannelsIdentical() {
		channelCount = 1
	}

	flags := uint8(0)
	if channelCount == 3 {
		flags |= isoIsMultiChannelMask
	}
	if m.UseBaseColorSpace {
		flags |= isoUseBaseColorMask
	}
	if m.BackwardDirection {
		flags |= 4
	}

	denom := m.BaseHdrHeadroomD
	useCommon := true
	if m.BaseHdrHeadroomD != denom || m.AltHdrHeadroomD != denom {
		useCommon = false
	}
	for c := 0; c < int(channelCount); c++ {
		if m.GainMapMinD[c] != denom || m.GainMapMaxD[c] != denom || m.GainMapGammaD[c] != denom ||
			m.BaseOffsetD[c] != denom || m.AltOffsetD[c] != denom {
			useCommon = false
		}
	}
	if useCommon {
		flags |= 8
	}

	out := make([]byte, 0, 128)
	writeU16 := func(v uint16) {
		out = append(out, byte(v>>8), byte(v))
	}
	writeU32 := func(v uint32) {
		out = append(out, byte(v>>24), byte(v>>16), byte(v>>8), byte(v))
	}
	writeS32 := func(v int32) {
		writeU32(uint32(v))
	}
	writeU8 := func(v uint8) {
		out = append(out, v)
	}

	writeU16(minVersion)
	writeU16(writerVersion)
	writeU8(flags)

	if useCommon {
		writeU32(denom)
		writeU32(m.BaseHdrHeadroomN)
		writeU32(m.AltHdrHeadroomN)
		for c := 0; c < int(channelCount); c++ {
			writeS32(m.GainMapMinN[c])
			writeS32(m.GainMapMaxN[c])
			writeU32(m.GainMapGammaN[c])
			writeS32(m.BaseOffsetN[c])
			writeS32(m.AltOffsetN[c])
		}
		return out, nil
	}

	writeU32(m.BaseHdrHeadroomN)
	writeU32(m.BaseHdrHeadroomD)
	writeU32(m.AltHdrHeadroomN)
	writeU32(m.AltHdrHeadroomD)
	for c := 0; c < int(channelCount); c++ {
		writeS32(m.GainMapMinN[c])
		writeU32(m.GainMapMinD[c])
		writeS32(m.GainMapMaxN[c])
		writeU32(m.GainMapMaxD[c])
		writeU32(m.GainMapGammaN[c])
		writeU32(m.GainMapGammaD[c])
		writeS32(m.BaseOffsetN[c])
		writeU32(m.BaseOffsetD[c])
		writeS32(m.AltOffsetN[c])
		writeU32(m.AltOffsetD[c])
	}
	return out, nil
}

func fracToFloat(from *gainmapMetadataFrac, to *GainMapMetadata) {
	to.UseBaseCG = from.UseBaseColorSpace
	for i := 0; i < 3; i++ {
		to.MinContentBoost[i] = exp2f(float32(from.GainMapMinN[i]) / float32(from.GainMapMinD[i]))
		to.MaxContentBoost[i] = exp2f(float32(from.GainMapMaxN[i]) / float32(from.GainMapMaxD[i]))
		to.Gamma[i] = float32(from.GainMapGammaN[i]) / float32(from.GainMapGammaD[i])
		to.OffsetSDR[i] = float32(from.BaseOffsetN[i]) / float32(from.BaseOffsetD[i])
		to.OffsetHDR[i] = float32(from.AltOffsetN[i]) / float32(from.AltOffsetD[i])
	}
	to.HDRCapacityMin = exp2f(float32(from.BaseHdrHeadroomN) / float32(from.BaseHdrHeadroomD))
	to.HDRCapacityMax = exp2f(float32(from.AltHdrHeadroomN) / float32(from.AltHdrHeadroomD))
}

func gainmapMetadataFloatToFraction(from *GainMapMetadata, to *gainmapMetadataFrac) error {
	if from == nil || to == nil {
		return errors.New("gainmap metadata missing")
	}
	to.BackwardDirection = false
	to.UseBaseColorSpace = from.UseBaseCG

	channelCount := 3
	if metaAllChannelsIdentical(from) {
		channelCount = 1
	}

	for i := 0; i < channelCount; i++ {
		if err := floatToSignedFraction(log2f(from.MaxContentBoost[i]), &to.GainMapMaxN[i], &to.GainMapMaxD[i]); err != nil {
			return err
		}
		if err := floatToSignedFraction(log2f(from.MinContentBoost[i]), &to.GainMapMinN[i], &to.GainMapMinD[i]); err != nil {
			return err
		}
		if err := floatToUnsignedFraction(from.Gamma[i], &to.GainMapGammaN[i], &to.GainMapGammaD[i]); err != nil {
			return err
		}
		if err := floatToSignedFraction(from.OffsetSDR[i], &to.BaseOffsetN[i], &to.BaseOffsetD[i]); err != nil {
			return err
		}
		if err := floatToSignedFraction(from.OffsetHDR[i], &to.AltOffsetN[i], &to.AltOffsetD[i]); err != nil {
			return err
		}
	}

	if channelCount == 1 {
		to.GainMapMaxN[2], to.GainMapMaxN[1] = to.GainMapMaxN[0], to.GainMapMaxN[0]
		to.GainMapMaxD[2], to.GainMapMaxD[1] = to.GainMapMaxD[0], to.GainMapMaxD[0]
		to.GainMapMinN[2], to.GainMapMinN[1] = to.GainMapMinN[0], to.GainMapMinN[0]
		to.GainMapMinD[2], to.GainMapMinD[1] = to.GainMapMinD[0], to.GainMapMinD[0]
		to.GainMapGammaN[2], to.GainMapGammaN[1] = to.GainMapGammaN[0], to.GainMapGammaN[0]
		to.GainMapGammaD[2], to.GainMapGammaD[1] = to.GainMapGammaD[0], to.GainMapGammaD[0]
		to.BaseOffsetN[2], to.BaseOffsetN[1] = to.BaseOffsetN[0], to.BaseOffsetN[0]
		to.BaseOffsetD[2], to.BaseOffsetD[1] = to.BaseOffsetD[0], to.BaseOffsetD[0]
		to.AltOffsetN[2], to.AltOffsetN[1] = to.AltOffsetN[0], to.AltOffsetN[0]
		to.AltOffsetD[2], to.AltOffsetD[1] = to.AltOffsetD[0], to.AltOffsetD[0]
	}

	if err := floatToUnsignedFraction(log2f(from.HDRCapacityMin), &to.BaseHdrHeadroomN, &to.BaseHdrHeadroomD); err != nil {
		return err
	}
	if err := floatToUnsignedFraction(log2f(from.HDRCapacityMax), &to.AltHdrHeadroomN, &to.AltHdrHeadroomD); err != nil {
		return err
	}
	return nil
}

func metaAllChannelsIdentical(m *GainMapMetadata) bool {
	if m == nil {
		return true
	}
	eq := func(a, b float32) bool { return a == b }
	for i := 1; i < 3; i++ {
		if !eq(m.MinContentBoost[0], m.MinContentBoost[i]) ||
			!eq(m.MaxContentBoost[0], m.MaxContentBoost[i]) ||
			!eq(m.Gamma[0], m.Gamma[i]) ||
			!eq(m.OffsetSDR[0], m.OffsetSDR[i]) ||
			!eq(m.OffsetHDR[0], m.OffsetHDR[i]) {
			return false
		}
	}
	return true
}

func (m *gainmapMetadataFrac) allChannelsIdentical() bool {
	return m.GainMapMinN[0] == m.GainMapMinN[1] && m.GainMapMinN[0] == m.GainMapMinN[2] &&
		m.GainMapMinD[0] == m.GainMapMinD[1] && m.GainMapMinD[0] == m.GainMapMinD[2] &&
		m.GainMapMaxN[0] == m.GainMapMaxN[1] && m.GainMapMaxN[0] == m.GainMapMaxN[2] &&
		m.GainMapMaxD[0] == m.GainMapMaxD[1] && m.GainMapMaxD[0] == m.GainMapMaxD[2] &&
		m.GainMapGammaN[0] == m.GainMapGammaN[1] && m.GainMapGammaN[0] == m.GainMapGammaN[2] &&
		m.GainMapGammaD[0] == m.GainMapGammaD[1] && m.GainMapGammaD[0] == m.GainMapGammaD[2] &&
		m.BaseOffsetN[0] == m.BaseOffsetN[1] && m.BaseOffsetN[0] == m.BaseOffsetN[2] &&
		m.BaseOffsetD[0] == m.BaseOffsetD[1] && m.BaseOffsetD[0] == m.BaseOffsetD[2] &&
		m.AltOffsetN[0] == m.AltOffsetN[1] && m.AltOffsetN[0] == m.AltOffsetN[2] &&
		m.AltOffsetD[0] == m.AltOffsetD[1] && m.AltOffsetD[0] == m.AltOffsetD[2]
}

func floatToSignedFraction(v float32, numerator *int32, denominator *uint32) error {
	const maxInt32 = int32(^uint32(0) >> 1)
	num, den, ok := floatToUnsignedFractionImpl(math.Abs(float64(v)), uint32(maxInt32))
	if !ok {
		return errors.New("failed to encode signed fraction")
	}
	n := int32(num)
	if v < 0 {
		n = -n
	}
	*numerator = n
	*denominator = den
	return nil
}

func floatToUnsignedFraction(v float32, numerator *uint32, denominator *uint32) error {
	const maxUint32 = ^uint32(0)
	num, den, ok := floatToUnsignedFractionImpl(float64(v), maxUint32)
	if !ok {
		return errors.New("failed to encode unsigned fraction")
	}
	*numerator = num
	*denominator = den
	return nil
}

func floatToUnsignedFractionImpl(v float64, maxNumerator uint32) (uint32, uint32, bool) {
	if math.IsNaN(v) || v < 0 || v > float64(maxNumerator) {
		return 0, 0, false
	}
	var maxD uint64
	if v <= 1 {
		maxD = uint64(^uint32(0))
	} else {
		maxD = uint64(math.Floor(float64(maxNumerator) / v))
	}

	den := uint32(1)
	prevD := uint32(0)
	currentV := v - math.Floor(v)
	const maxIter = 39
	for iter := 0; iter < maxIter; iter++ {
		numeratorDouble := float64(den) * v
		if numeratorDouble > float64(maxNumerator) {
			return 0, 0, false
		}
		num := uint32(math.Round(numeratorDouble))
		if math.Abs(numeratorDouble-float64(num)) == 0.0 {
			return num, den, true
		}
		if currentV == 0 {
			return num, den, true
		}
		currentV = 1.0 / currentV
		newD := float64(prevD) + math.Floor(currentV)*float64(den)
		if newD > float64(maxD) {
			return num, den, true
		}
		prevD = den
		if newD > float64(^uint32(0)) {
			return 0, 0, false
		}
		den = uint32(newD)
		currentV -= math.Floor(currentV)
	}
	num := uint32(math.Round(float64(den) * v))
	return num, den, true
}
