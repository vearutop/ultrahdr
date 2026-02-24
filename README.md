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
uhdrtool resize -in testdata/uhdr.jpg -out testdata/uhdr_thumb.jpg -w 2400 -h 1600 -q 85 -gq 75 \
  -primary-out testdata/uhdr_thumb_primary.jpg -gainmap-out testdata/uhdr_thumb_gainmap.jpg

# split into components + metadata bundle
uhdrtool split -in testdata/uhdr.jpg \
  -primary-out primary.jpg -gainmap-out gainmap.jpg -meta-out meta.json

# join without the original template
uhdrtool join -meta meta.json -primary primary.jpg -gainmap gainmap.jpg -out out.jpg

# rebase on a better SDR (approximate gainmap adjustment)
uhdrtool rebase -in testdata/uhdr.jpg -primary better_sdr.jpg -out better_uhdr.jpg

# rebase using HDR EXR (new gainmap generation)
uhdrtool rebase -primary sdr.jpg -exr hdr.exr -out output.jpg

# rebase using HDR TIFF (new gainmap generation)
uhdrtool rebase -primary sdr.jpg -tiff hdr.tif -out output.jpg

# detect UltraHDR
uhdrtool detect -in testdata/uhdr.jpg
```

## Resizing

Primary image interpolation is built in. Set `ResizeSpec.Interpolation` to one of
`InterpolationNearest`, `InterpolationBilinear`, `InterpolationBicubic`,
`InterpolationMitchellNetravali`, `InterpolationLanczos2`, or `InterpolationLanczos3`. Gainmap
resizing uses the same interpolation mode.

`ResizeHDR` and `ResizeSDR` accept one or more `ResizeSpec` entries and deliver outputs via
`ReceiveResult`. `ResizeHDR` also supports `ReceiveSplit` to inspect container metadata before
resizing.
`ResizeSpec.Crop` optionally crops the source before resizing (for UltraHDR, the gainmap is cropped
to the corresponding region automatically).

## Compatibility

- Google Pixel UltraHDR JPEG/R files that store gainmap metadata in XMP only (no secondary ISO
  APP2) are supported. Resize/rebase regenerates ISO 21496-1 metadata for the gainmap JPEG to
  preserve HDR rendering in Chrome.
- Older Adobe Camera Raw UltraHDR files are supported when gainmap XMP values are encoded as
  `rdf:Seq` (`<rdf:li>...`) entries instead of attribute values.
- Containers with embedded JPEG thumbnails are handled using MPF image ranges, so split/resize
  targets the correct primary and gainmap images.
- Rebase applies ICC-aware gamut alignment for sRGB, Display P3, and Adobe RGB primaries before
  gainmap math.

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

## ResizeSDR

```go
f, err := os.Open("input.jpg")
if err != nil {
	// handle error
}
defer f.Close()
var resized *ultrahdr.Result
err := ultrahdr.ResizeSDR(f, ultrahdr.ResizeSpec{
  Width:         1600,
  Height:        1200,
  Quality:       85,
  Interpolation: ultrahdr.InterpolationLanczos2,
  KeepMeta:      true,
  ReceiveResult: func(res *ultrahdr.Result, err error) { resized = res },
})
if err != nil {
	// handle error
}
_ = os.WriteFile("output.jpg", resized.Primary, 0o644)
```

`ResizeSDR` behavior:
- `KeepMeta=true`: preserves EXIF/ICC (including Display P3 and Adobe RGB profiles).
- `KeepMeta=false`: strips metadata and converts Display P3/Adobe RGB input to sRGB pixels for
  web-safe output.

- `ResizeSDR` accepts multiple `ResizeSpec` entries and performs a single source decode.
- Each spec receives a result via its `ReceiveResult` callback.


## Join

```go
primary, _ := os.ReadFile("primary.jpg")
gainmap, _ := os.ReadFile("gainmap.jpg")
container, err := ultrahdr.Join(primary, gainmap, nil, nil)
if err != nil {
  // handle error
}
_ = os.WriteFile("out.jpg", container, 0o644)
```

If you have a split result, you can reuse its metadata:

```go
f, _ := os.Open("template.jpg")
split, _ := ultrahdr.Split(f)
container, _ := ultrahdr.Join(primary, gainmap, nil, split)
```

## Limitations

- SDR base image is assumed to be sRGB.
- HDR image input is assumed to be linear RGB relative to SDR white.
- Gain map sampling uses nearest-neighbor.
- Only XMP + ISO 21496-1 gain map metadata are generated.
- `ResizeSDR` metadata preservation is limited to EXIF and ICC segments (XMP is not preserved).
- Full ICC color management is not implemented; only sRGB/Display P3/Adobe RGB primary profile
  handling is applied in rebase and metadata-stripped ResizeSDR output.
