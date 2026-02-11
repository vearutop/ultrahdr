package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"

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
	fmt.Fprintln(os.Stderr, "  detect -in input.jpg")
	fmt.Fprintln(os.Stderr, "  split  -in input.jpg -primary-out primary.jpg -gainmap-out gainmap.jpg [-meta-out meta.json]")
	fmt.Fprintln(os.Stderr, "  join   -meta meta.json -primary primary.jpg -gainmap gainmap.jpg -out output.jpg")
	fmt.Fprintln(os.Stderr, "        (or) join -template input.jpg -primary primary.jpg -gainmap gainmap.jpg -out output.jpg")
}

func runResize(args []string) error {
	fs := flag.NewFlagSet("resize", flag.ContinueOnError)
	inPath := fs.String("in", "", "input UltraHDR JPEG")
	outPath := fs.String("out", "", "output UltraHDR JPEG")
	width := fs.Int("w", 0, "target width")
	height := fs.Int("h", 0, "target height")
	q := fs.Int("q", 85, "base quality")
	gq := fs.Int("gq", 75, "gainmap quality")
	primaryOut := fs.String("primary-out", "", "write primary JPEG")
	gainmapOut := fs.String("gainmap-out", "", "write gainmap JPEG")
	fs.SetOutput(os.Stderr)
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *inPath == "" || *outPath == "" || *width <= 0 || *height <= 0 {
		return errors.New("missing required arguments")
	}
	return ultrahdr.ResizeUltraHDRFile(*inPath, *outPath, *width, *height, func(opt *ultrahdr.ResizeOptions) {
		opt.PrimaryQuality = *q
		opt.GainmapQuality = *gq
		opt.PrimaryOut = *primaryOut
		opt.GainmapOut = *gainmapOut
	})
}

func runRebase(args []string) error {
	fs := flag.NewFlagSet("rebase", flag.ContinueOnError)
	inPath := fs.String("in", "", "input UltraHDR JPEG")
	primaryPath := fs.String("primary", "", "new SDR JPEG")
	outPath := fs.String("out", "", "output UltraHDR JPEG")
	q := fs.Int("q", 95, "base quality")
	gq := fs.Int("gq", 85, "gainmap quality")
	primaryOut := fs.String("primary-out", "", "write primary JPEG")
	gainmapOut := fs.String("gainmap-out", "", "write gainmap JPEG")
	fs.SetOutput(os.Stderr)
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *inPath == "" || *primaryPath == "" || *outPath == "" {
		return errors.New("missing required arguments")
	}
	opts := &ultrahdr.RebaseOptions{
		BaseQuality:    *q,
		GainmapQuality: *gq,
	}
	return ultrahdr.RebaseUltraHDRFile(*inPath, *primaryPath, *outPath, opts, *primaryOut, *gainmapOut)
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
	f, err := os.Open(filepath.Clean(*inPath))
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
		return fmt.Errorf("missing required arguments")
	}
	data, err := os.ReadFile(filepath.Clean(*inPath))
	if err != nil {
		return err
	}
	split, err := ultrahdr.Split(data)
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Clean(*primaryOut), split.PrimaryJPEG, 0o644); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Clean(*gainmapOut), split.GainmapJPEG, 0o644); err != nil {
		return err
	}
	if *metaOut != "" {
		bundle, err := ultrahdr.BuildMetadataBundle(split.PrimaryJPEG, split.Segs)
		if err != nil {
			return err
		}
		payload, err := json.MarshalIndent(bundle, "", "  ")
		if err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Clean(*metaOut), payload, 0o644); err != nil {
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
		return fmt.Errorf("missing required arguments")
	}
	primary, err := os.ReadFile(filepath.Clean(*primaryPath))
	if err != nil {
		return err
	}
	gainmap, err := os.ReadFile(filepath.Clean(*gainmapPath))
	if err != nil {
		return err
	}
	if *metaPath != "" {
		metaData, err := os.ReadFile(filepath.Clean(*metaPath))
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
		return os.WriteFile(filepath.Clean(*outPath), container, 0o644)
	}
	if *templatePath == "" {
		return fmt.Errorf("missing -meta or -template")
	}
	template, err := os.ReadFile(filepath.Clean(*templatePath))
	if err != nil {
		return err
	}
	split, err := ultrahdr.Split(template)
	if err != nil {
		return err
	}
	exif, icc, err := ultrahdr.ExtractExifAndIcc(primary)
	if err != nil {
		return err
	}
	if len(exif) == 0 && len(icc) == 0 {
		exif, icc, err = ultrahdr.ExtractExifAndIcc(template)
		if err != nil {
			return err
		}
	}
	container, err := ultrahdr.AssembleContainerVipsLike(primary, gainmap, exif, icc, split.Segs.SecondaryXMP, split.Segs.SecondaryISO)
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Clean(*outPath), container, 0o644)
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "error:", err)
	os.Exit(1)
}
