// Package authority implements the field authority chain that decides
// whether a proposed label correction is trustworthy enough to apply.
//
// The chain ranks evidence sources from weakest to strongest:
//
//	filesystem < container < bitstream < sidecar
//
// File-system hints (file names, directory layout) can describe a library
// but never author a correction: they are excluded from the admissible set.
// Every proposed value must agree with the strongest admissible evidence
// after canonical normalization, otherwise it is rejected for manual review.
package authority

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"
)

// languageAliases maps ISO 639-2/3 and legacy English names to the canonical
// ISO 639-1 primary subtag used throughout the tool.
var languageAliases = map[string]string{
	"eng": "en", "english": "en",
	"zho": "zh", "chi": "zh", "cmn": "zh", "chinese": "zh",
	"jpn": "ja", "japanese": "ja",
	"kor": "ko", "korean": "ko",
	"fra": "fr", "fre": "fr", "french": "fr",
	"deu": "de", "ger": "de", "german": "de",
	"spa": "es", "spanish": "es",
	"ita": "it", "italian": "it",
	"rus": "ru", "russian": "ru",
	"por": "pt", "portuguese": "pt",
	"nld": "nl", "dut": "nl", "dutch": "nl",
	"ara": "ar", "arabic": "ar",
	"hin": "hi", "hindi": "hi",
	"vie": "vi", "vietnamese": "vi",
	"tha": "th", "thai": "th",
	"ind": "id", "indonesian": "id",
}

// NormalizeLanguage reduces a raw language tag to its canonical primary
// subtag. Region and script are dropped so "en", "en-US" and "eng" compare
// equal. The original spelling is retained in the evidence provenance.
func NormalizeLanguage(raw string) (string, error) {
	tag := strings.ToLower(strings.TrimSpace(raw))
	if tag == "" {
		return "", fmt.Errorf("empty language tag")
	}
	tag = strings.ReplaceAll(tag, "_", "-")
	primary := strings.SplitN(tag, "-", 2)[0]
	if canonical, ok := languageAliases[primary]; ok {
		primary = canonical
	}
	if !isAlpha(primary) {
		return "", fmt.Errorf("invalid language tag %q", raw)
	}
	return primary, nil
}

func isAlpha(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < 'a' || r > 'z' {
			return false
		}
	}
	return true
}

// ParseDuration accepts clock time ("MM:SS", "HH:MM:SS.mmm"), a Go duration
// ("1h2m3.5s") or a bare number of seconds, and returns seconds.
func ParseDuration(raw string) (float64, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return 0, fmt.Errorf("empty duration")
	}
	if seconds, err := strconv.ParseFloat(value, 64); err == nil {
		if seconds < 0 {
			return 0, fmt.Errorf("negative duration %q", raw)
		}
		return seconds, nil
	}
	if d, err := time.ParseDuration(value); err == nil {
		return d.Seconds(), nil
	}
	if seconds, ok := parseClock(value); ok {
		return seconds, nil
	}
	return 0, fmt.Errorf("unparseable duration %q", raw)
}

func parseClock(value string) (float64, bool) {
	parts := strings.Split(value, ":")
	if len(parts) < 2 || len(parts) > 3 {
		return 0, false
	}
	var total float64
	for i, part := range parts {
		number, err := strconv.ParseFloat(strings.TrimSpace(part), 64)
		if err != nil || number < 0 {
			return 0, false
		}
		if i < len(parts)-1 && number != math.Trunc(number) {
			return 0, false
		}
		total = total*60 + number
	}
	return total, true
}

// CanonicalDuration renders seconds as a stable millisecond-precision string.
func CanonicalDuration(seconds float64) string {
	rounded := math.Round(seconds*1000) / 1000
	return strconv.FormatFloat(rounded, 'f', -1, 64) + "s"
}

// DurationEqual compares two second values within epsilon.
func DurationEqual(a, b, epsilon float64) bool {
	return math.Abs(a-b) <= epsilon
}
