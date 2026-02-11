package ultrahdr

import (
	"encoding/binary"
	"errors"
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
