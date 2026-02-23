// Package main is a command-line tool for working with UltraHDR JPEGs.
package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"image"
	"os"

	"github.com/vearutop/ultrahdr"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	switch os.Args[1] {
	case "resize":
		if err := runResize(os.Args[2:]); err != nil {
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
	fmt.Fprintln(os.Stderr, "  resize -in input.jpg -out output.jpg -w 2400 -h 1600 [-q 85] [-gq 75] [-primary-out p.jpg] [-gainmap-out g.jpg]")
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
	interpMode := ultrahdr.InterpolationNearest
	switch *interp {
	case "nearest":
		interpMode = ultrahdr.InterpolationNearest
	case "bilinear":
		interpMode = ultrahdr.InterpolationBilinear
	case "bicubic":
		interpMode = ultrahdr.InterpolationBicubic
	case "mitchell":
		interpMode = ultrahdr.InterpolationMitchellNetravali
	case "lanczos2":
		interpMode = ultrahdr.InterpolationLanczos2
	case "lanczos3":
		interpMode = ultrahdr.InterpolationLanczos3
	}
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
		bundle, err := ultrahdr.BuildMetadataBundle(split.Primary, split.Segs)
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
		container, err := ultrahdr.AssembleFromBundle(primary, gainmap, &bundle)
		if err != nil {
			return err
		}
		return os.WriteFile(*outPath, container, 0o644)
	}
	if *templatePath == "" {
		secondaryXMP, secondaryISO, err := ultrahdr.ExtractGainmapMetadataSegments(gainmap)
		if err != nil {
			return err
		}
		if len(secondaryXMP) == 0 && len(secondaryISO) == 0 {
			return errors.New("missing gainmap metadata (use -meta, -template, or embed XMP/ISO in gainmap)")
		}
		exif, icc, err := ultrahdr.ExtractEXIFAndICC(primary)
		if err != nil {
			return err
		}
		if len(exif) == 0 && len(icc) == 0 {
			exif, icc, err = ultrahdr.ExtractEXIFAndICC(gainmap)
			if err != nil {
				return err
			}
		}
		container, err := ultrahdr.AssembleContainer(primary, gainmap, exif, icc, secondaryXMP, secondaryISO)
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
	exif, icc, err := ultrahdr.ExtractEXIFAndICC(primary)
	if err != nil {
		return err
	}
	if len(exif) == 0 && len(icc) == 0 {
		exif, icc, err = ultrahdr.ExtractEXIFAndICC(split.Primary)
		if err != nil {
			return err
		}
	}
	container, err := ultrahdr.AssembleContainer(primary, gainmap, exif, icc, split.Segs.SecondaryXMP, split.Segs.SecondaryISO)
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
