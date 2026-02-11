package ultrahdr

import "encoding/binary"

const (
	mpfNumPictures = 2
	mpfEndianSize  = 4
	mpfTagCount    = 3
	mpfTagSize     = 12

	mpfTypeLong      = 0x4
	mpfTypeUndefined = 0x7

	mpfVersionTag          = 0xB000
	mpfVersionCount        = 4
	mpfNumberOfImagesTag   = 0xB001
	mpfNumberOfImagesCount = 1
	mpfEntryTag            = 0xB002
	mpfEntrySize           = 16

	mpfAttrFormatJpeg  = 0x0000000
	mpfAttrTypePrimary = 0x030000
)

var (
	mpfSig       = []byte{'M', 'P', 'F', 0}
	mpfBigEndian = []byte{0x4D, 0x4D, 0x00, 0x2A}
	mpfVersion   = []byte{'0', '1', '0', '0'}
)

func calculateMpfSize() int {
	return len(mpfSig) + mpfEndianSize + 4 + 2 + mpfTagCount*mpfTagSize + 4 + mpfNumPictures*mpfEntrySize
}

func generateMpf(primarySize, primaryOffset, secondarySize, secondaryOffset int) []byte {
	buf := make([]byte, 0, calculateMpfSize())
	putU16 := func(v uint16) { tmp := make([]byte, 2); binary.BigEndian.PutUint16(tmp, v); buf = append(buf, tmp...) }
	putU32 := func(v uint32) { tmp := make([]byte, 4); binary.BigEndian.PutUint32(tmp, v); buf = append(buf, tmp...) }

	buf = append(buf, mpfSig...)
	buf = append(buf, mpfBigEndian...)

	indexIfdOffset := uint32(mpfEndianSize + len(mpfSig))
	putU32(indexIfdOffset)

	putU16(mpfTagCount)

	// Version tag
	putU16(mpfVersionTag)
	putU16(mpfTypeUndefined)
	putU32(mpfVersionCount)
	buf = append(buf, mpfVersion...)

	// Number of images
	putU16(mpfNumberOfImagesTag)
	putU16(mpfTypeLong)
	putU32(mpfNumberOfImagesCount)
	putU32(mpfNumPictures)

	// MP entries
	putU16(mpfEntryTag)
	putU16(mpfTypeUndefined)
	putU32(mpfEntrySize * mpfNumPictures)
	// Offset from TIFF header start (after MPF signature).
	mpEntryOffset := uint32(8 + 2 + mpfTagCount*mpfTagSize + 4)
	putU32(mpEntryOffset)

	// Attribute IFD offset (zero)
	putU32(0)

	// Primary entry
	putU32(mpfAttrFormatJpeg | mpfAttrTypePrimary)
	putU32(uint32(primarySize))
	putU32(uint32(primaryOffset))
	putU16(0)
	putU16(0)

	// Secondary entry
	putU32(mpfAttrFormatJpeg)
	putU32(uint32(secondarySize))
	putU32(uint32(secondaryOffset))
	putU16(0)
	putU16(0)

	return buf
}
