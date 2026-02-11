package ultrahdr

import (
	"errors"
	"regexp"
	"strconv"
	"strings"
)

var (
	reVersion    = regexp.MustCompile(`hdrgm:Version="([^"]+)"`)
	reGainMapMin = regexp.MustCompile(`hdrgm:GainMapMin="([^"]+)"`)
	reGainMapMax = regexp.MustCompile(`hdrgm:GainMapMax="([^"]+)"`)
	reGamma      = regexp.MustCompile(`hdrgm:Gamma="([^"]+)"`)
	reOffsetSDR  = regexp.MustCompile(`hdrgm:OffsetSDR="([^"]+)"`)
	reOffsetHDR  = regexp.MustCompile(`hdrgm:OffsetHDR="([^"]+)"`)
	reHDRCapMin  = regexp.MustCompile(`hdrgm:HDRCapacityMin="([^"]+)"`)
	reHDRCapMax  = regexp.MustCompile(`hdrgm:HDRCapacityMax="([^"]+)"`)
	reBaseIsHDR  = regexp.MustCompile(`hdrgm:BaseRenditionIsHDR="([^"]+)"`)
)

func parseXMP(app1 []byte) (*GainMapMetadata, error) {
	if len(app1) < len(xmpNamespace)+2 {
		return nil, errors.New("xmp block too small")
	}
	if !strings.HasPrefix(string(app1), xmpNamespace+"\x00") {
		return nil, errors.New("xmp namespace mismatch")
	}
	xml := string(app1[len(xmpNamespace)+1:])

	meta := &GainMapMetadata{Version: jpegrVersion, UseBaseCG: true}
	meta.MinContentBoost[0] = 1
	meta.MaxContentBoost[0] = 1
	meta.Gamma[0] = 1
	meta.OffsetSDR[0] = 1.0 / 64.0
	meta.OffsetHDR[0] = 1.0 / 64.0
	meta.HDRCapacityMin = 1
	meta.HDRCapacityMax = 1

	getStr := func(re *regexp.Regexp) (string, bool) {
		m := re.FindStringSubmatch(xml)
		if len(m) != 2 {
			return "", false
		}
		return m[1], true
	}
	getFloat := func(re *regexp.Regexp) (float32, bool, error) {
		str, ok := getStr(re)
		if !ok {
			return 0, false, nil
		}
		v, err := strconv.ParseFloat(str, 32)
		if err != nil {
			return 0, true, err
		}
		return float32(v), true, nil
	}

	if v, ok := getStr(reVersion); ok {
		meta.Version = v
	} else {
		return nil, errors.New("xmp missing version")
	}

	if v, ok, err := getFloat(reGainMapMax); err != nil {
		return nil, err
	} else if ok {
		meta.MaxContentBoost[0] = exp2f(v)
	} else {
		return nil, errors.New("xmp missing GainMapMax")
	}

	if v, ok, err := getFloat(reHDRCapMax); err != nil {
		return nil, err
	} else if ok {
		meta.HDRCapacityMax = exp2f(v)
	} else {
		return nil, errors.New("xmp missing HDRCapacityMax")
	}

	if v, ok, err := getFloat(reGainMapMin); err != nil {
		return nil, err
	} else if ok {
		meta.MinContentBoost[0] = exp2f(v)
	}
	if v, ok, err := getFloat(reGamma); err != nil {
		return nil, err
	} else if ok {
		meta.Gamma[0] = v
	}
	if v, ok, err := getFloat(reOffsetSDR); err != nil {
		return nil, err
	} else if ok {
		meta.OffsetSDR[0] = v
	}
	if v, ok, err := getFloat(reOffsetHDR); err != nil {
		return nil, err
	} else if ok {
		meta.OffsetHDR[0] = v
	}
	if v, ok, err := getFloat(reHDRCapMin); err != nil {
		return nil, err
	} else if ok {
		meta.HDRCapacityMin = exp2f(v)
	}
	if v, ok := getStr(reBaseIsHDR); ok {
		if v == "True" {
			return nil, errors.New("base rendition HDR not supported")
		}
	}

	for i := 1; i < 3; i++ {
		if meta.MinContentBoost[i] == 0 {
			meta.MinContentBoost[i] = meta.MinContentBoost[0]
		}
		if meta.MaxContentBoost[i] == 0 {
			meta.MaxContentBoost[i] = meta.MaxContentBoost[0]
		}
		if meta.Gamma[i] == 0 {
			meta.Gamma[i] = meta.Gamma[0]
		}
		if meta.OffsetSDR[i] == 0 {
			meta.OffsetSDR[i] = meta.OffsetSDR[0]
		}
		if meta.OffsetHDR[i] == 0 {
			meta.OffsetHDR[i] = meta.OffsetHDR[0]
		}
	}
	return meta, nil
}
