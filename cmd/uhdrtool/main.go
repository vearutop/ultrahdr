// Package main is a command-line tool for working with UltraHDR JPEGs.
package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"image"
	"image/color"
	"io"
	"os"

	"github.com/vearutop/ultrahdr"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	switch os.Args[1] {
	case "crop":
		if err := runCrop(os.Args[2:]); err != nil {
			fail(err)
		}
	case "resize":
		if err := runResize(os.Args[2:]); err != nil {
			fail(err)
		}
	case "grid":
		if err := runGrid(os.Args[2:]); err != nil {
			fail(err)
		}
	case "rebase":
		if err := runRebase(os.Args[2:]); err != nil {
			fail(err)
		}
	case "detect":
		if err := runDetect(os.Args[2:]); err != nil {
			fail(err)
		}
	case "split":
		if err := runSplit(os.Args[2:]); err != nil {
			fail(err)
		}
	case "join":
		if err := runJoin(os.Args[2:]); err != nil {
			fail(err)
		}
	case "gmstats":
		if err := runGainmapStats(os.Args[2:]); err != nil {
			fail(err)
		}
	default:
		usage()
		os.Exit(2)
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, "Usage: uhdrtool <command> [args]")
	fmt.Fprintln(os.Stderr, "Commands:")
	fmt.Fprintln(os.Stderr, "  crop  -in input.jpg -out output.jpg -x 0 -y 0 -w 800 -h 600 [-tw 800] [-th 600] [-q 85] [-gq 75] [-keep-meta]")
	fmt.Fprintln(os.Stderr, "  resize -in input.jpg -out output.jpg -w 2400 -h 1600 [-q 85] [-gq 75] [-primary-out p.jpg] [-gainmap-out g.jpg]")
	fmt.Fprintln(os.Stderr, "  grid  -in a.jpg -in b.jpg -cols 2 -cell-w 400 -cell-h 300 -out grid.jpg [-q 85] [-bg #000000] [-interp lanczos2]")
	fmt.Fprintln(os.Stderr, "  rebase -in uhdr.jpg -primary better_sdr.jpg -out output.jpg [-q 95] [-gq 85] [-primary-out p.jpg] [-gainmap-out g.jpg]")
	fmt.Fprintln(os.Stderr, "  rebase -primary sdr.jpg -exr hdr.exr -out output.jpg [-q 95] [-gq 85] [-primary-out p.jpg] [-gainmap-out g.jpg]")
	fmt.Fprintln(os.Stderr, "  rebase -primary sdr.jpg -tiff hdr.tif -out output.jpg [-q 95] [-gq 85] [-primary-out p.jpg] [-gainmap-out g.jpg]")
	fmt.Fprintln(os.Stderr, "  detect -in input.jpg")
	fmt.Fprintln(os.Stderr, "  split  -in input.jpg -primary-out primary.jpg -gainmap-out gainmap.jpg [-meta-out meta.json]")
	fmt.Fprintln(os.Stderr, "  join   -meta meta.json -primary primary.jpg -gainmap gainmap.jpg -out output.jpg")
	fmt.Fprintln(os.Stderr, "        (or) join -template input.jpg -primary primary.jpg -gainmap gainmap.jpg -out output.jpg")
	fmt.Fprintln(os.Stderr, "        (or) join -primary primary.jpg -gainmap gainmap.jpg -out output.jpg")
	fmt.Fprintln(os.Stderr, "  gmstats -in gainmap.jpg")
}

func runCrop(args []string) error {
	fs := flag.NewFlagSet("crop", flag.ContinueOnError)
	inPath := fs.String("in", "", "input JPEG")
	outPath := fs.String("out", "", "output JPEG")
	x := fs.Int("x", 0, "crop x")
	y := fs.Int("y", 0, "crop y")
	w := fs.Int("w", 0, "crop width")
	h := fs.Int("h", 0, "crop height")
	targetW := fs.Uint("tw", 0, "target width (default crop width)")
	targetH := fs.Uint("th", 0, "target height (default crop height)")
	q := fs.Int("q", 85, "base quality")
	gq := fs.Int("gq", 75, "gainmap quality")
	keepMeta := fs.Bool("keep-meta", false, "keep SDR metadata (EXIF/ICC)")
	interp := fs.String("interp", "lanczos2", "resize interpolation method, one of: nearest, bilinear, bicubic, mitchell, lanczos2, lanczos3")
	fs.SetOutput(os.Stderr)
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *inPath == "" || *outPath == "" || *w <= 0 || *h <= 0 {
		return errors.New("missing required arguments")
	}
	if *targetW == 0 {
		*targetW = uint(*w)
	}
	if *targetH == 0 {
		*targetH = uint(*h)
	}

	rect := image.Rect(*x, *y, *x+*w, *y+*h)
	interpMode := parseInterpolation(*interp)

	f, err := os.Open(*inPath)
	if err != nil {
		return err
	}
	defer f.Close()

	ok, err := ultrahdr.IsUltraHDR(f)
	if err != nil {
		return err
	}
	if _, err := f.Seek(0, 0); err != nil {
		return err
	}

	if ok {
		var out *ultrahdr.Result
		err = ultrahdr.ResizeHDR(f, ultrahdr.ResizeSpec{
			Width:          *targetW,
			Height:         *targetH,
			Crop:           &rect,
			Quality:        *q,
			GainmapQuality: *gq,
			Interpolation:  interpMode,
			ReceiveResult: func(res *ultrahdr.Result, err error) {
				if err == nil {
					out = res
				}
			},
		})
		if err != nil {
			return err
		}
		if out == nil || out.Container == nil {
			return errors.New("crop produced no output")
		}
		return os.WriteFile(*outPath, out.Container, 0o644)
	}

	var out *ultrahdr.Result
	err = ultrahdr.ResizeSDR(f, ultrahdr.ResizeSpec{
		Width:         *targetW,
		Height:        *targetH,
		Crop:          &rect,
		Quality:       *q,
		Interpolation: interpMode,
		KeepMeta:      *keepMeta,
		ReceiveResult: func(res *ultrahdr.Result, err error) {
			if err == nil {
				out = res
			}
		},
	})
	if err != nil {
		return err
	}
	if out == nil || out.Primary == nil {
		return errors.New("crop produced no output")
	}
	return os.WriteFile(*outPath, out.Primary, 0o644)
}

func runResize(args []string) error {
	fs := flag.NewFlagSet("resize", flag.ContinueOnError)
	inPath := fs.String("in", "", "input UltraHDR JPEG")
	outPath := fs.String("out", "", "output UltraHDR JPEG")
	width := fs.Uint("w", 0, "target width")
	height := fs.Uint("h", 0, "target height")
	q := fs.Int("q", 85, "base quality")
	gq := fs.Int("gq", 75, "gainmap quality")
	primaryOut := fs.String("primary-out", "", "write primary JPEG")
	gainmapOut := fs.String("gainmap-out", "", "write gainmap JPEG")
	interp := fs.String("interp", "lanczos2", "resize interpolation method, one of: nearest, bilinear, bicubic, mitchell, lanczos2, lanczos3")
	fs.SetOutput(os.Stderr)
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *inPath == "" || *outPath == "" || *width <= 0 || *height <= 0 {
		return errors.New("missing required arguments")
	}
	f, err := os.Open(*inPath)
	if err != nil {
		return err
	}
	defer f.Close()
	interpMode := parseInterpolation(*interp)
	var resized *ultrahdr.Result
	err = ultrahdr.ResizeHDR(f, ultrahdr.ResizeSpec{
		Width:          *width,
		Height:         *height,
		Quality:        *q,
		GainmapQuality: *gq,
		Interpolation:  interpMode,
		ReceiveResult: func(res *ultrahdr.Result, err error) {
			if err == nil {
				resized = res
			}
		},
	})
	if err != nil {
		return err
	}
	if resized == nil {
		return errors.New("resize produced no output")
	}
	if err := os.WriteFile(*outPath, resized.Container, 0o644); err != nil {
		return err
	}
	if *primaryOut != "" {
		if err := os.WriteFile(*primaryOut, resized.Primary, 0o644); err != nil {
			return err
		}
	}
	if *gainmapOut != "" {
		if err := os.WriteFile(*gainmapOut, resized.Gainmap, 0o644); err != nil {
			return err
		}
	}
	return nil
}

func runGrid(args []string) error {
	fs := flag.NewFlagSet("grid", flag.ContinueOnError)
	var inputs multiFlag
	fs.Var(&inputs, "in", "input image (repeat for multiple)")
	cols := fs.Int("cols", 0, "number of columns")
	cellW := fs.Int("cell-w", 0, "cell width")
	cellH := fs.Int("cell-h", 0, "cell height")
	outPath := fs.String("out", "", "output JPEG")
	q := fs.Int("q", 85, "base quality")
	bg := fs.String("bg", "", "background color (#RRGGBB or r,g,b)")
	interp := fs.String("interp", "lanczos2", "resize interpolation method, one of: nearest, bilinear, bicubic, mitchell, lanczos2, lanczos3")
	primaryOut := fs.String("primary-out", "", "write primary JPEG")
	gainmapOut := fs.String("gainmap-out", "", "write gainmap JPEG")
	fs.SetOutput(os.Stderr)
	if err := fs.Parse(args); err != nil {
		return err
	}
	if len(fs.Args()) > 0 {
		inputs = append(inputs, fs.Args()...)
	}
	if len(inputs) == 0 || *cols <= 0 || *cellW <= 0 || *cellH <= 0 || *outPath == "" {
		return errors.New("missing required arguments")
	}

	files := make([]*os.File, 0, len(inputs))
	readers := make([]io.Reader, 0, len(inputs))
	for _, p := range inputs {
		f, err := os.Open(p)
		if err != nil {
			return err
		}
		files = append(files, f)
		readers = append(readers, f)
	}
	for _, f := range files {
		defer f.Close()
	}

	var bgColor color.Color
	if *bg != "" {
		parsed, err := parseColor(*bg)
		if err != nil {
			return err
		}
		bgColor = parsed
	}

	res, err := ultrahdr.Grid(readers, *cols, *cellW, *cellH, &ultrahdr.GridOptions{
		Quality:       *q,
		Interpolation: parseInterpolation(*interp),
		Background:    bgColor,
	})
	if err != nil {
		return err
	}
	if res == nil || res.Primary == nil {
		return errors.New("grid produced no output")
	}
	if err := os.WriteFile(*outPath, res.Container, 0o644); err != nil {
		return err
	}
	if *primaryOut != "" {
		if err := os.WriteFile(*primaryOut, res.Primary, 0o644); err != nil {
			return err
		}
	}
	if *gainmapOut != "" && res.Gainmap != nil {
		if err := os.WriteFile(*gainmapOut, res.Gainmap, 0o644); err != nil {
			return err
		}
	}
	return nil
}

func runRebase(args []string) error {
	fs := flag.NewFlagSet("rebase", flag.ContinueOnError)
	inPath := fs.String("in", "", "input UltraHDR JPEG")
	primaryPath := fs.String("primary", "", "new SDR JPEG")
	exrPath := fs.String("exr", "", "HDR OpenEXR input")
	tiffPath := fs.String("tiff", "", "HDR TIFF input")
	outPath := fs.String("out", "", "output UltraHDR JPEG")
	q := fs.Int("q", 95, "base quality")
	gq := fs.Int("gq", 85, "gainmap quality")
	primaryOut := fs.String("primary-out", "", "write primary JPEG")
	gainmapOut := fs.String("gainmap-out", "", "write gainmap JPEG")
	fs.SetOutput(os.Stderr)
	if err := fs.Parse(args); err != nil {
		return err
	}
	var opts []ultrahdr.RebaseOption
	if *q > 0 {
		opts = append(opts, ultrahdr.WithBaseQuality(*q))
	}
	if *gq > 0 {
		opts = append(opts, ultrahdr.WithGainmapQuality(*gq))
	}
	if *primaryOut != "" {
		opts = append(opts, ultrahdr.WithPrimaryOut(*primaryOut))
	}
	if *gainmapOut != "" {
		opts = append(opts, ultrahdr.WithGainmapOut(*gainmapOut))
	}
	if *exrPath != "" && *tiffPath != "" {
		return errors.New("use only one of -exr or -tiff")
	}
	if *exrPath != "" {
		if *primaryPath == "" || *outPath == "" {
			return errors.New("missing required arguments")
		}
		return ultrahdr.RebaseFromEXRFile(*primaryPath, *exrPath, *outPath, opts...)
	}
	if *tiffPath != "" {
		if *primaryPath == "" || *outPath == "" {
			return errors.New("missing required arguments")
		}
		return ultrahdr.RebaseFromTIFFFile(*primaryPath, *tiffPath, *outPath, opts...)
	}
	if *inPath == "" || *primaryPath == "" || *outPath == "" {
		return errors.New("missing required arguments")
	}
	return ultrahdr.RebaseFile(*inPath, *primaryPath, *outPath, opts...)
}

func runDetect(args []string) error {
	fs := flag.NewFlagSet("detect", flag.ContinueOnError)
	inPath := fs.String("in", "", "input JPEG")
	fs.SetOutput(os.Stderr)
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *inPath == "" {
		return errors.New("missing required arguments")
	}
	f, err := os.Open(*inPath)
	if err != nil {
		return err
	}
	defer f.Close()
	ok, err := ultrahdr.IsUltraHDR(f)
	if err != nil {
		return err
	}
	if ok {
		fmt.Fprintln(os.Stdout, "ultrahdr")
		return nil
	}
	fmt.Fprintln(os.Stdout, "not ultrahdr")
	return nil
}

func runSplit(args []string) error {
	fs := flag.NewFlagSet("split", flag.ContinueOnError)
	inPath := fs.String("in", "", "input UltraHDR JPEG")
	primaryOut := fs.String("primary-out", "", "primary output JPEG")
	gainmapOut := fs.String("gainmap-out", "", "gainmap output JPEG")
	metaOut := fs.String("meta-out", "", "metadata json output")
	fs.SetOutput(os.Stderr)
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *inPath == "" || *primaryOut == "" || *gainmapOut == "" {
		return errors.New("missing required arguments")
	}
	f, err := os.Open(*inPath)
	if err != nil {
		return err
	}
	defer f.Close()
	split, err := ultrahdr.Split(f)
	if err != nil {
		return err
	}
	if err := os.WriteFile(*primaryOut, split.Primary, 0o644); err != nil {
		return err
	}
	if err := os.WriteFile(*gainmapOut, split.Gainmap, 0o644); err != nil {
		return err
	}
	if *metaOut != "" {
		bundle, err := split.BuildMetadataBundle()
		if err != nil {
			return err
		}
		payload, err := json.MarshalIndent(bundle, "", "  ")
		if err != nil {
			return err
		}
		if err := os.WriteFile(*metaOut, payload, 0o644); err != nil {
			return err
		}
	}
	return nil
}

func runJoin(args []string) error {
	fs := flag.NewFlagSet("join", flag.ContinueOnError)
	templatePath := fs.String("template", "", "template UltraHDR JPEG for metadata")
	metaPath := fs.String("meta", "", "metadata json")
	primaryPath := fs.String("primary", "", "primary JPEG")
	gainmapPath := fs.String("gainmap", "", "gainmap JPEG")
	outPath := fs.String("out", "", "output UltraHDR JPEG")
	fs.SetOutput(os.Stderr)
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *primaryPath == "" || *gainmapPath == "" || *outPath == "" {
		return errors.New("missing required arguments")
	}
	primary, err := os.ReadFile(*primaryPath)
	if err != nil {
		return err
	}
	gainmap, err := os.ReadFile(*gainmapPath)
	if err != nil {
		return err
	}
	if *metaPath != "" {
		metaData, err := os.ReadFile(*metaPath)
		if err != nil {
			return err
		}
		var bundle ultrahdr.MetadataBundle
		if err := json.Unmarshal(metaData, &bundle); err != nil {
			return err
		}
		container, err := ultrahdr.Join(primary, gainmap, &bundle, nil)
		if err != nil {
			return err
		}
		return os.WriteFile(*outPath, container, 0o644)
	}
	if *templatePath == "" {
		container, err := ultrahdr.Join(primary, gainmap, nil, nil)
		if err != nil {
			return err
		}
		return os.WriteFile(*outPath, container, 0o644)
	}
	template, err := os.Open(*templatePath)
	if err != nil {
		return err
	}
	defer template.Close()
	split, err := ultrahdr.Split(template)
	if err != nil {
		return err
	}
	container, err := ultrahdr.Join(primary, gainmap, nil, split)
	if err != nil {
		return err
	}
	return os.WriteFile(*outPath, container, 0o644)
}

func runGainmapStats(args []string) error {
	fs := flag.NewFlagSet("gmstats", flag.ContinueOnError)
	inPath := fs.String("in", "", "gainmap JPEG")
	fs.SetOutput(os.Stderr)
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *inPath == "" {
		return errors.New("missing required arguments")
	}
	data, err := os.ReadFile(*inPath)
	if err != nil {
		return err
	}
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return err
	}
	b := img.Bounds()
	if b.Empty() {
		return errors.New("empty image")
	}
	minR, minG, minB := uint8(255), uint8(255), uint8(255)
	maxR, maxG, maxB := uint8(0), uint8(0), uint8(0)
	var sumR, sumG, sumB uint64
	var count uint64
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			r, g, b2, _ := img.At(x, y).RGBA()
			r8 := uint8(r >> 8)
			g8 := uint8(g >> 8)
			b8 := uint8(b2 >> 8)
			if r8 < minR {
				minR = r8
			}
			if g8 < minG {
				minG = g8
			}
			if b8 < minB {
				minB = b8
			}
			if r8 > maxR {
				maxR = r8
			}
			if g8 > maxG {
				maxG = g8
			}
			if b8 > maxB {
				maxB = b8
			}
			sumR += uint64(r8)
			sumG += uint64(g8)
			sumB += uint64(b8)
			count++
		}
	}
	avgR := float64(sumR) / float64(count)
	avgG := float64(sumG) / float64(count)
	avgB := float64(sumB) / float64(count)
	fmt.Fprintf(os.Stdout, "min=%d,%d,%d max=%d,%d,%d avg=%.2f,%.2f,%.2f\n", minR, minG, minB, maxR, maxG, maxB, avgR, avgG, avgB)
	return nil
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "error:", err)
	os.Exit(1)
}

type multiFlag []string

func (m *multiFlag) String() string {
	if m == nil {
		return ""
	}
	return fmt.Sprintf("%v", []string(*m))
}

func (m *multiFlag) Set(value string) error {
	*m = append(*m, value)
	return nil
}

func parseInterpolation(name string) ultrahdr.Interpolation {
	switch name {
	case "nearest":
		return ultrahdr.InterpolationNearest
	case "bilinear":
		return ultrahdr.InterpolationBilinear
	case "bicubic":
		return ultrahdr.InterpolationBicubic
	case "mitchell":
		return ultrahdr.InterpolationMitchellNetravali
	case "lanczos2":
		return ultrahdr.InterpolationLanczos2
	case "lanczos3":
		return ultrahdr.InterpolationLanczos3
	default:
		return ultrahdr.InterpolationNearest
	}
}

func parseColor(value string) (color.Color, error) {
	if value == "" {
		return nil, errors.New("empty color")
	}
	if value[0] == '#' {
		if len(value) != 7 {
			return nil, errors.New("invalid hex color")
		}
		v, err := parseHexColor(value[1:])
		if err != nil {
			return nil, err
		}
		return v, nil
	}
	var r, g, b int
	if _, err := fmt.Sscanf(value, "%d,%d,%d", &r, &g, &b); err != nil {
		return nil, errors.New("invalid color format")
	}
	if r < 0 || r > 255 || g < 0 || g > 255 || b < 0 || b > 255 {
		return nil, errors.New("color out of range")
	}
	return color.NRGBA{R: uint8(r), G: uint8(g), B: uint8(b), A: 0xFF}, nil
}

func parseHexColor(hex string) (color.Color, error) {
	var r, g, b uint8
	if _, err := fmt.Sscanf(hex, "%02x%02x%02x", &r, &g, &b); err != nil {
		if _, err := fmt.Sscanf(hex, "%02X%02X%02X", &r, &g, &b); err != nil {
			return nil, errors.New("invalid hex color")
		}
	}
	return color.NRGBA{R: r, G: g, B: b, A: 0xFF}, nil
}
