# ultrahdr (pure Go)

This is a minimal, pure-Go port of libultrahdr focused on correctness and portability.

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

## Custom Resizer

You can plug in a custom resizer (e.g. `github.com/nfnt/resize`) via `ResizeOptions.Resizer`.
The resizer is used for both base and gainmap. Gainmap resizing is performed in linear gain
space and then re-encoded to preserve HDR correctness.

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

## Limitations

- SDR base image is assumed to be sRGB.
- HDR image input is assumed to be linear RGB relative to SDR white.
- No gamut conversion or transfer function conversion.
- Gain map sampling uses nearest-neighbor.
- Only XMP + ISO 21496-1 gain map metadata are generated.
