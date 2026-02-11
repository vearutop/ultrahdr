// Package ultrahdr provides a pure-Go implementation of the UltraHDR JPEG/R format.
//
// This is a pragmatic port focused on correctness and portability rather than performance.
// It uses the patched standard image/jpeg package for JPEG encode/decode and assembles/parses
// the JPEG/R container (MPF + XMP + ISO 21496-1 gain map metadata) in Go.
package ultrahdr
