# ultrahdr (pure Go)

This is a minimal, pure-Go port of libultrahdr focused on correctness and portability.

## Why?

UltraHDR is an emerging format of presenting true HDR images with a great level of compatibility with legacy apps and devices. 
If you're interested in this topic, check https://gregbenzphotography.com/hdr/ for more context.

Because this technology is a relatively new, and is built on top of another relatively new (HDR displays) technology, tooling landscape is pretty sparse now.

Existing solutions for Go require CGO builds with non-trivial dependencies and environment requirements.

This project started as an experiment with `codex` LLM tool, with human steering and testing it turned out to be a success. 
Resulting code leverages Go's cross-compiltion and powers statically built binaries with no depencies, it also bridges the way to new features like `uhdrtool rebase`. 

Initial development log (colored output in terminal):
```
curl -s https://raw.githubusercontent.com/vearutop/ultrahdr/refs/heads/master/testdata/codex.log
```

## Usage

```go
package main

import (
	"image"
	"image/jpeg"
	"os"

	"github.com/vearutop/ultrahdr"
)

func main() {
	// Load SDR base image.
	f, _ := os.Open("sdr.jpg")
	sdrImg, _, _ := image.Decode(f)
	f.Close()

	// Build HDR image data (linear RGB, 1.0 = SDR white).
	// This example is a placeholder; fill hdr.Pix with real data.
	hdr := &ultrahdr.HDRImage{Width: sdrImg.Bounds().Dx(), Height: sdrImg.Bounds().Dy(), Stride: sdrImg.Bounds().Dx() * 3, Pix: make([]float32, sdrImg.Bounds().Dx()*sdrImg.Bounds().Dy()*3)}

	// Encode JPEG/R.
	jpegrBytes, meta, _ := ultrahdr.Encode(hdr, sdrImg, &ultrahdr.EncodeOptions{Quality: 95})
	_ = meta
	_ = os.WriteFile("out.jpegr.jpg", jpegrBytes, 0644)

	// Decode JPEG/R.
	data, _ := os.ReadFile("out.jpegr.jpg")
	hdrOut, sdrOut, metaOut, _ := ultrahdr.Decode(data, &ultrahdr.DecodeOptions{MaxDisplayBoost: 4})
	_, _ = hdrOut, metaOut

	outFile, _ := os.Create("base.jpg")
	_ = jpeg.Encode(outFile, sdrOut, &jpeg.Options{Quality: 95})
	outFile.Close()
}
```

## CLI

```bash
# resize UltraHDR (writes container + components)
go run ./cmd/uhdrtool resize -in testdata/uhdr.jpg -out testdata/uhdr_thumb.jpg -w 2400 -h 1600 -q 85 -gq 75 \
  -primary-out testdata/uhdr_thumb_primary.jpg -gainmap-out testdata/uhdr_thumb_gainmap.jpg

# split into components + metadata bundle
go run ./cmd/uhdrtool split -in testdata/uhdr.jpg \
  -primary-out primary.jpg -gainmap-out gainmap.jpg -meta-out meta.json

# join without the original template
go run ./cmd/uhdrtool join -meta meta.json -primary primary.jpg -gainmap gainmap.jpg -out out.jpg

# rebase on a better SDR (approximate gainmap adjustment)
go run ./cmd/uhdrtool rebase -in testdata/uhdr.jpg -primary better_sdr.jpg -out better_uhdr.jpg

# detect UltraHDR
go run ./cmd/uhdrtool detect -in testdata/uhdr.jpg
```

## Resizing

Primary image interpolation is built in. Set `ResizeOptions.PrimaryInterpolation` to one of
`InterpolationNearest`, `InterpolationBilinear`, `InterpolationBicubic`,
`InterpolationMitchellNetravali`, `InterpolationLanczos2`, or `InterpolationLanczos3`. Gainmap
resizing uses the same interpolation mode.

## Compatibility

- Google Pixel UltraHDR JPEG/R files that store gainmap metadata in XMP only (no secondary ISO
  APP2) are supported. Resize/rebase regenerates ISO 21496-1 metadata for the gainmap JPEG to
  preserve HDR rendering in Chrome.
- Older Adobe Camera Raw UltraHDR files are supported when gainmap XMP values are encoded as
  `rdf:Seq` (`<rdf:li>...`) entries instead of attribute values.
- Containers with embedded JPEG thumbnails are handled using MPF image ranges, so split/resize
  targets the correct primary and gainmap images.
- Rebase applies ICC-aware gamut alignment for sRGB and Display P3 primaries before gainmap math.

## Detection

```go
f, err := os.Open("image.jpg")
if err != nil {
	// handle error
}
defer f.Close()

ok, err := ultrahdr.IsUltraHDR(f)
// ok == true means the file looks like a valid UltraHDR JPEG/R container.
```

## ResizeJPEG (SDR)

```go
data, err := os.ReadFile("input.jpg")
if err != nil {
	// handle error
}
resized, err := ultrahdr.ResizeJPEG(data, 1600, 1200, 85, ultrahdr.InterpolationLanczos2, true)
if err != nil {
	// handle error
}
_ = os.WriteFile("output.jpg", resized, 0o644)
```

## Limitations

- SDR base image is assumed to be sRGB.
- HDR image input is assumed to be linear RGB relative to SDR white.
- No gamut conversion or transfer function conversion.
- Gain map sampling uses nearest-neighbor.
- Only XMP + ISO 21496-1 gain map metadata are generated.
- `ResizeJPEG` metadata preservation is limited to EXIF and ICC segments (XMP is not preserved).
- Full ICC color management is not implemented; only sRGB/Display P3 primary gamut alignment is
  applied in rebase.
