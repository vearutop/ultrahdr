package ultrahdr

import (
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

var (
	reVersion       = regexp.MustCompile(`hdrgm:Version="([^"]+)"`)
	reGainMapMin    = regexp.MustCompile(`hdrgm:GainMapMin="([^"]+)"`)
	reGainMapMax    = regexp.MustCompile(`hdrgm:GainMapMax="([^"]+)"`)
	reGamma         = regexp.MustCompile(`hdrgm:Gamma="([^"]+)"`)
	reOffsetSDR     = regexp.MustCompile(`hdrgm:OffsetSDR="([^"]+)"`)
	reOffsetHDR     = regexp.MustCompile(`hdrgm:OffsetHDR="([^"]+)"`)
	reHDRCapMin     = regexp.MustCompile(`hdrgm:HDRCapacityMin="([^"]+)"`)
	reHDRCapMax     = regexp.MustCompile(`hdrgm:HDRCapacityMax="([^"]+)"`)
	reBaseIsHDR     = regexp.MustCompile(`hdrgm:BaseRenditionIsHDR="([^"]+)"`)
	reGainMapMinSeq = regexp.MustCompile(`(?s)<hdrgm:GainMapMin>.*?<rdf:Seq>(.*?)</rdf:Seq>.*?</hdrgm:GainMapMin>`)
	reGainMapMaxSeq = regexp.MustCompile(`(?s)<hdrgm:GainMapMax>.*?<rdf:Seq>(.*?)</rdf:Seq>.*?</hdrgm:GainMapMax>`)
	reGammaSeq      = regexp.MustCompile(`(?s)<hdrgm:Gamma>.*?<rdf:Seq>(.*?)</rdf:Seq>.*?</hdrgm:Gamma>`)
	reRdfLi         = regexp.MustCompile(`(?s)<rdf:li>([^<]+)</rdf:li>`)
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
	getSeqFloats := func(re *regexp.Regexp) ([]float32, bool, error) {
		m := re.FindStringSubmatch(xml)
		if len(m) != 2 {
			return nil, false, nil
		}
		items := reRdfLi.FindAllStringSubmatch(m[1], -1)
		if len(items) == 0 {
			return nil, false, nil
		}
		out := make([]float32, 0, len(items))
		for _, it := range items {
			if len(it) != 2 {
				continue
			}
			v, err := strconv.ParseFloat(strings.TrimSpace(it[1]), 32)
			if err != nil {
				return nil, true, err
			}
			out = append(out, float32(v))
		}
		if len(out) == 0 {
			return nil, false, nil
		}
		return out, true, nil
	}

	applySeq := func(dst *[3]float32, vals []float32) {
		if len(vals) == 0 {
			return
		}
		if len(vals) == 1 {
			dst[0], dst[1], dst[2] = vals[0], vals[0], vals[0]
			return
		}
		dst[0] = vals[0]
		if len(vals) > 1 {
			dst[1] = vals[1]
		}
		if len(vals) > 2 {
			dst[2] = vals[2]
		}
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
	} else if seq, ok, err := getSeqFloats(reGainMapMaxSeq); err != nil {
		return nil, err
	} else if ok {
		var tmp [3]float32
		applySeq(&tmp, seq)
		for i := 0; i < 3; i++ {
			meta.MaxContentBoost[i] = exp2f(tmp[i])
		}
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
	} else if seq, ok, err := getSeqFloats(reGainMapMinSeq); err != nil {
		return nil, err
	} else if ok {
		var tmp [3]float32
		applySeq(&tmp, seq)
		for i := 0; i < 3; i++ {
			meta.MinContentBoost[i] = exp2f(tmp[i])
		}
	}
	if v, ok, err := getFloat(reGamma); err != nil {
		return nil, err
	} else if ok {
		meta.Gamma[0] = v
	} else if seq, ok, err := getSeqFloats(reGammaSeq); err != nil {
		return nil, err
	} else if ok {
		var tmp [3]float32
		applySeq(&tmp, seq)
		meta.Gamma = tmp
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

func buildGainmapXMP(meta *GainMapMetadata) []byte {
	if meta == nil {
		return nil
	}
	format := func(v float32) string {
		return strconv.FormatFloat(float64(v), 'g', 6, 32)
	}
	xml := fmt.Sprintf(
		`<x:xmpmeta xmlns:x="adobe:ns:meta/" x:xmptk="Adobe XMP Core 5.1.2"><rdf:RDF xmlns:rdf="http://www.w3.org/1999/02/22-rdf-syntax-ns#"><rdf:Description xmlns:hdrgm="http://ns.adobe.com/hdr-gain-map/1.0/" hdrgm:Version="%s" hdrgm:GainMapMin="%s" hdrgm:GainMapMax="%s" hdrgm:Gamma="%s" hdrgm:OffsetSDR="%s" hdrgm:OffsetHDR="%s" hdrgm:HDRCapacityMin="%s" hdrgm:HDRCapacityMax="%s" hdrgm:BaseRenditionIsHDR="False"/></rdf:RDF></x:xmpmeta>`,
		meta.Version,
		format(log2f(meta.MinContentBoost[0])),
		format(log2f(meta.MaxContentBoost[0])),
		format(meta.Gamma[0]),
		format(meta.OffsetSDR[0]),
		format(meta.OffsetHDR[0]),
		format(log2f(meta.HDRCapacityMin)),
		format(log2f(meta.HDRCapacityMax)),
	)
	out := make([]byte, 0, len(xmpNamespace)+1+len(xml))
	out = append(out, []byte(xmpNamespace)...)
	out = append(out, 0)
	out = append(out, xml...)
	return out
}

func buildPrimaryXMP(meta *GainMapMetadata, secondaryImageSize int) []byte {
	if meta == nil {
		return nil
	}
	xml := fmt.Sprintf(
		`<x:xmpmeta xmlns:x="adobe:ns:meta/" x:xmptk="Adobe XMP Core 5.1.2"><rdf:RDF xmlns:rdf="http://www.w3.org/1999/02/22-rdf-syntax-ns#"><rdf:Description xmlns:Container="http://ns.google.com/photos/1.0/container/" xmlns:Item="http://ns.google.com/photos/1.0/container/item/" xmlns:hdrgm="http://ns.adobe.com/hdr-gain-map/1.0/" hdrgm:Version="%s"><Container:Directory><rdf:Seq><rdf:li rdf:parseType="Resource"><Container:Item Item:Semantic="Primary" Item:Mime="image/jpeg"/></rdf:li><rdf:li rdf:parseType="Resource"><Container:Item Item:Semantic="GainMap" Item:Mime="image/jpeg" Item:Length="%d"/></rdf:li></rdf:Seq></Container:Directory></rdf:Description></rdf:RDF></x:xmpmeta>`,
		meta.Version,
		secondaryImageSize,
	)
	out := make([]byte, 0, len(xmpNamespace)+1+len(xml))
	out = append(out, []byte(xmpNamespace)...)
	out = append(out, 0)
	out = append(out, xml...)
	return out
}
