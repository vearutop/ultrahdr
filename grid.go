package ultrahdr

import (
	"bytes"
	"errors"
	"image"
	"image/color"
	"image/draw"
	"io"
	"math"
)

// GridOptions configures grid rendering for SDR inputs.
type GridOptions struct {
	Quality       int           // JPEG quality for the output (0 uses default).
	Interpolation Interpolation // Resize interpolation mode.
	Background    color.Color   // Background fill color (nil uses black).
}

// Grid builds a sprite grid from SDR images. Inputs are resized to fit each cell
// preserving aspect ratio and padded with black. Output is encoded as sRGB JPEG.
func Grid(readers []io.Reader, cols int, cellW, cellH int, opts *GridOptions) (*Result, error) {
	if len(readers) == 0 {
		return nil, errors.New("no input images")
	}
	if cols <= 0 {
		return nil, errors.New("invalid columns")
	}
	if cellW <= 0 || cellH <= 0 {
		return nil, errors.New("invalid cell size")
	}

	quality := defaultPrimaryQuality
	interp := InterpolationLanczos2
	if opts != nil {
		if opts.Quality > 0 {
			quality = opts.Quality
		}
		if opts.Interpolation != 0 {
			interp = opts.Interpolation
		}
	}

	rows := int(math.Ceil(float64(len(readers)) / float64(cols)))
	gridW := cols * cellW
	gridH := rows * cellH
	grid := image.NewNRGBA(image.Rect(0, 0, gridW, gridH))
	bg := color.NRGBA{A: 0xFF}
	if opts != nil && opts.Background != nil {
		bg = color.NRGBAModel.Convert(opts.Background).(color.NRGBA)
	}
	draw.Draw(grid, grid.Bounds(), &image.Uniform{C: bg}, image.Point{}, draw.Src)

	for idx, r := range readers {
		if r == nil {
			return nil, errors.New("nil reader")
		}
		data, err := io.ReadAll(r)
		if err != nil {
			return nil, err
		}
		img, _, err := image.Decode(bytes.NewReader(data))
		if err != nil {
			return nil, err
		}

		srcProfile := colorProfile{gamut: colorGamutSRGB, transfer: colorTransferSRGB}
		_, icc, err := extractExifAndIcc(data)
		if err == nil {
			srcProfile = detectColorProfileFromICCProfile(collectICCProfile(icc))
		}
		dstProfile := colorProfile{gamut: colorGamutSRGB, transfer: colorTransferSRGB}
		if srcProfile != dstProfile {
			img = convertImageProfile(img, srcProfile, dstProfile)
		}

		resized, w, h := resizeToFit(img, cellW, cellH, interp)
		col := idx % cols
		row := idx / cols
		x0 := col*cellW + (cellW-w)/2
		y0 := row*cellH + (cellH-h)/2
		dstRect := image.Rect(x0, y0, x0+w, y0+h)
		draw.Draw(grid, dstRect, resized, resized.Bounds().Min, draw.Src)
	}

	out, err := encodeWithQuality(grid, quality)
	if err != nil {
		return nil, err
	}
	return &Result{Container: out, Primary: out}, nil
}

func resizeToFit(img image.Image, cellW, cellH int, interp Interpolation) (image.Image, int, int) {
	b := img.Bounds()
	sw := b.Dx()
	sh := b.Dy()
	if sw <= 0 || sh <= 0 {
		return image.NewNRGBA(image.Rect(0, 0, 1, 1)), 1, 1
	}

	scaleX := float64(cellW) / float64(sw)
	scaleY := float64(cellH) / float64(sh)
	scale := scaleX
	if scaleY < scaleX {
		scale = scaleY
	}
	if scale <= 0 {
		scale = 1
	}

	newW := int(math.Round(float64(sw) * scale))
	newH := int(math.Round(float64(sh) * scale))
	if newW < 1 {
		newW = 1
	}
	if newH < 1 {
		newH = 1
	}
	if newW == sw && newH == sh {
		return img, newW, newH
	}
	resized := resizeImageInterpolated(img, newW, newH, interp)
	return resized, newW, newH
}
