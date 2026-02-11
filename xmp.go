package ultrahdr

import (
	"errors"
	"fmt"
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
		if err := parseXmpSeqValues(xml, meta); err != nil {
			return nil, err
		}
	}

	if v, ok, err := getFloat(reHDRCapMax); err != nil {
		return nil, err
	} else if ok {
		meta.HDRCapacityMax = exp2f(v)
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

func parseXmpSeqValues(xml string, meta *GainMapMetadata) error {
	if meta == nil {
		return errors.New("nil metadata")
	}
	if vals, ok, err := extractSeqValues(xml, "GainMapMin"); err != nil {
		return err
	} else if ok {
		assignSeqBoost(meta.MinContentBoost[:], vals, true)
	}
	if vals, ok, err := extractSeqValues(xml, "GainMapMax"); err != nil {
		return err
	} else if ok {
		assignSeqBoost(meta.MaxContentBoost[:], vals, true)
	} else {
		return errors.New("xmp missing GainMapMax")
	}
	if vals, ok, err := extractSeqValues(xml, "Gamma"); err != nil {
		return err
	} else if ok {
		assignSeqFloat(meta.Gamma[:], vals)
	}
	if vals, ok, err := extractSeqValues(xml, "OffsetSDR"); err != nil {
		return err
	} else if ok {
		assignSeqFloat(meta.OffsetSDR[:], vals)
	}
	if vals, ok, err := extractSeqValues(xml, "OffsetHDR"); err != nil {
		return err
	} else if ok {
		assignSeqFloat(meta.OffsetHDR[:], vals)
	}
	if v, ok, err := extractSingleValue(xml, "HDRCapacityMin"); err != nil {
		return err
	} else if ok {
		meta.HDRCapacityMin = exp2f(v)
	}
	if v, ok, err := extractSingleValue(xml, "HDRCapacityMax"); err != nil {
		return err
	} else if ok {
		meta.HDRCapacityMax = exp2f(v)
	} else {
		return errors.New("xmp missing HDRCapacityMax")
	}
	if v, ok := extractStringValue(xml, "BaseRenditionIsHDR"); ok {
		if v == "True" || v == "true" || v == "1" {
			return errors.New("base rendition HDR not supported")
		}
	}
	return nil
}

func extractSeqValues(xml string, tag string) ([]float32, bool, error) {
	reTag := regexp.MustCompile(`(?s)<hdrgm:` + tag + `[^>]*>(.*?)</hdrgm:` + tag + `>`)
	m := reTag.FindStringSubmatch(xml)
	if len(m) != 2 {
		return nil, false, nil
	}
	reLi := regexp.MustCompile(`<rdf:li>([^<]+)</rdf:li>`)
	matches := reLi.FindAllStringSubmatch(m[1], -1)
	if len(matches) == 0 {
		return nil, false, nil
	}
	vals := make([]float32, 0, len(matches))
	for _, sm := range matches {
		if len(sm) != 2 {
			continue
		}
		v, err := strconv.ParseFloat(strings.TrimSpace(sm[1]), 32)
		if err != nil {
			return nil, false, err
		}
		vals = append(vals, float32(v))
	}
	return vals, true, nil
}

func extractSingleValue(xml string, tag string) (float32, bool, error) {
	reTag := regexp.MustCompile(`(?s)hdrgm:` + tag + `="([^"]+)"`)
	if m := reTag.FindStringSubmatch(xml); len(m) == 2 {
		v, err := strconv.ParseFloat(m[1], 32)
		if err != nil {
			return 0, false, err
		}
		return float32(v), true, nil
	}
	vals, ok, err := extractSeqValues(xml, tag)
	if err != nil || !ok || len(vals) == 0 {
		return 0, false, err
	}
	return vals[0], true, nil
}

func extractStringValue(xml string, tag string) (string, bool) {
	reTag := regexp.MustCompile(`(?s)hdrgm:` + tag + `="([^"]+)"`)
	if m := reTag.FindStringSubmatch(xml); len(m) == 2 {
		return strings.TrimSpace(m[1]), true
	}
	reElem := regexp.MustCompile(`(?s)<hdrgm:` + tag + `[^>]*>([^<]+)</hdrgm:` + tag + `>`)
	if m := reElem.FindStringSubmatch(xml); len(m) == 2 {
		return strings.TrimSpace(m[1]), true
	}
	return "", false
}

func assignSeqBoost(dst []float32, vals []float32, log2Values bool) {
	if len(vals) == 0 || len(dst) == 0 {
		return
	}
	count := len(vals)
	if count > len(dst) {
		count = len(dst)
	}
	for i := 0; i < count; i++ {
		if log2Values {
			dst[i] = exp2f(vals[i])
		} else {
			dst[i] = vals[i]
		}
	}
}

func assignSeqFloat(dst []float32, vals []float32) {
	if len(vals) == 0 || len(dst) == 0 {
		return
	}
	count := len(vals)
	if count > len(dst) {
		count = len(dst)
	}
	for i := 0; i < count; i++ {
		dst[i] = vals[i]
	}
}

func generateXmpPrimary(secondaryLength int, meta *GainMapMetadata) []byte {
	b := strings.Builder{}
	b.WriteString("<x:xmpmeta xmlns:x=\"adobe:ns:meta/\" x:xmptk=\"Adobe XMP Core 5.1.2\">")
	b.WriteString("<rdf:RDF xmlns:rdf=\"http://www.w3.org/1999/02/22-rdf-syntax-ns#\">")
	b.WriteString("<rdf:Description xmlns:Container=\"http://ns.google.com/photos/1.0/container/\"")
	b.WriteString(" xmlns:Item=\"http://ns.google.com/photos/1.0/container/item/\"")
	b.WriteString(" xmlns:hdrgm=\"http://ns.adobe.com/hdr-gain-map/1.0/\"")
	b.WriteString(" hdrgm:Version=\"")
	b.WriteString(meta.Version)
	b.WriteString("\">")
	b.WriteString("<Container:Directory><rdf:Seq>")
	b.WriteString("<rdf:li rdf:parseType=\"Resource\"><Container:Item Item:Semantic=\"Primary\" Item:Mime=\"image/jpeg\"/></rdf:li>")
	b.WriteString("<rdf:li rdf:parseType=\"Resource\"><Container:Item Item:Semantic=\"GainMap\" Item:Mime=\"image/jpeg\" Item:Length=\"")
	b.WriteString(fmt.Sprintf("%d", secondaryLength))
	b.WriteString("\"/></rdf:li>")
	b.WriteString("</rdf:Seq></Container:Directory>")
	b.WriteString("</rdf:Description></rdf:RDF></x:xmpmeta>")
	return []byte(b.String())
}

func generateXmpSecondary(meta *GainMapMetadata) []byte {
	b := strings.Builder{}
	b.WriteString("<x:xmpmeta xmlns:x=\"adobe:ns:meta/\" x:xmptk=\"Adobe XMP Core 5.1.2\">")
	b.WriteString("<rdf:RDF xmlns:rdf=\"http://www.w3.org/1999/02/22-rdf-syntax-ns#\">")
	b.WriteString("<rdf:Description xmlns:hdrgm=\"http://ns.adobe.com/hdr-gain-map/1.0/\"")
	b.WriteString(" hdrgm:Version=\"")
	b.WriteString(meta.Version)
	b.WriteString("\"")
	b.WriteString(fmt.Sprintf(" hdrgm:GainMapMin=\"%g\"", log2f(meta.MinContentBoost[0])))
	b.WriteString(fmt.Sprintf(" hdrgm:GainMapMax=\"%g\"", log2f(meta.MaxContentBoost[0])))
	b.WriteString(fmt.Sprintf(" hdrgm:Gamma=\"%g\"", meta.Gamma[0]))
	b.WriteString(fmt.Sprintf(" hdrgm:OffsetSDR=\"%g\"", meta.OffsetSDR[0]))
	b.WriteString(fmt.Sprintf(" hdrgm:OffsetHDR=\"%g\"", meta.OffsetHDR[0]))
	b.WriteString(fmt.Sprintf(" hdrgm:HDRCapacityMin=\"%g\"", log2f(meta.HDRCapacityMin)))
	b.WriteString(fmt.Sprintf(" hdrgm:HDRCapacityMax=\"%g\"", log2f(meta.HDRCapacityMax)))
	b.WriteString(" hdrgm:BaseRenditionIsHDR=\"False\"/>")
	b.WriteString("</rdf:RDF></x:xmpmeta>")
	return []byte(b.String())
}
