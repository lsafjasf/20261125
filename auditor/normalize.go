package auditor

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"
)

// languageAliases maps common ISO 639-2/3 and legacy codes to the canonical
// ISO 639-1 primary subtag used inside this package.
var languageAliases = map[string]string{
	"eng":      "en",
	"english":  "en",
	"zho":      "zh",
	"chi":      "zh",
	"cmn":      "zh",
	"jpn":      "ja",
	"japanese": "ja",
	"kor":      "ko",
	"fra":      "fr",
	"fre":      "fr",
	"french":   "fr",
	"deu":      "de",
	"ger":      "de",
	"german":   "de",
	"spa":      "es",
	"spanish":  "es",
	"ita":      "it",
	"rus":      "ru",
	"por":      "pt",
	"nld":      "nl",
	"dut":      "nl",
	"ara":      "ar",
	"hin":      "hi",
	"pol":      "pl",
	"tur":      "tr",
	"vie":      "vi",
	"tha":      "th",
	"ind":      "id",
	"ukr":      "uk",
	"chinese":  "zh",
}

// NormalizeLanguage reduces a raw language tag to its canonical primary
// subtag. Region and script are intentionally dropped so that "en", "en-US"
// and "eng" are treated as the same language; the raw value is always kept
// in the provenance, so the dropped parts remain auditable.
func NormalizeLanguage(raw string) (string, error) {
	tag := strings.TrimSpace(raw)
	if tag == "" {
		return "", fmt.Errorf("empty language tag")
	}
	tag = strings.ToLower(tag)
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
		if !(r >= 'a' && r <= 'z') {
			return false
		}
	}
	return true
}

// ParseDuration accepts three notations and returns the duration in seconds:
//   - clock time "HH:MM:SS" or "MM:SS" with an optional decimal fraction;
//   - Go duration such as "1h2m3.5s" or "6123.5s";
//   - a bare number interpreted as seconds ("6123.5").
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
	if seconds, ok := parseClockDuration(value); ok {
		return seconds, nil
	}
	return 0, fmt.Errorf("unparseable duration %q", raw)
}

func parseClockDuration(value string) (float64, bool) {
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

// CanonicalDuration renders seconds as a stable canonical string, rounded to
// millisecond precision.
func CanonicalDuration(seconds float64) string {
	rounded := math.Round(seconds*1000) / 1000
	return strconv.FormatFloat(rounded, 'f', -1, 64) + "s"
}
