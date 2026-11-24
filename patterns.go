package main

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

// ColorPatch is a named HLG code-value highlight. The same code values are
// sent through every display configuration.
type ColorPatch struct {
	Name string `json:"name"`
	Code RGB    `json:"code"`
}

func HighlightLevels() []float64 {
	return []float64{0.75, 0.85, 0.95, 1.0, 1.05}
}

func HighlightPatches(level float64) []ColorPatch {
	at := func(r, g, b float64) RGB {
		return RGB{r * level, g * level, b * level}
	}
	return []ColorPatch{
		{Name: "白", Code: at(1, 1, 1)},
		{Name: "黄", Code: at(1, 0.86, 0.08)},
		{Name: "肤", Code: at(1, 0.72, 0.48)},
		{Name: "红", Code: at(1, 0.04, 0.03)},
		{Name: "绿", Code: at(0.08, 1, 0.18)},
		{Name: "青", Code: at(0.05, 0.88, 1)},
		{Name: "蓝", Code: at(0.08, 0.20, 1)},
		{Name: "品红", Code: at(1, 0.10, 0.78)},
	}
}

func GrayCodes(count int) []float64 {
	if count < 2 {
		count = 2
	}
	out := make([]float64, count)
	for i := range out {
		out[i] = float64(i) / float64(count-1)
	}
	return out
}

func ParsePeaks(text string) ([]float64, error) {
	parts := strings.Split(text, ",")
	peaks := make([]float64, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		value, err := strconv.ParseFloat(part, 64)
		if err != nil || value <= 0 || math.IsNaN(value) || math.IsInf(value, 0) {
			return nil, fmt.Errorf("invalid peak %q", part)
		}
		peaks = append(peaks, value)
	}
	if len(peaks) == 0 {
		return nil, fmt.Errorf("at least one positive peak is required")
	}
	return peaks, nil
}

func formatNits(v float64) string {
	if math.Abs(v-math.Round(v)) < 0.05 {
		return fmt.Sprintf("%.0f nit", v)
	}
	return fmt.Sprintf("%.1f nit", v)
}

func formatNitValue(v float64) string {
	if v >= 100 {
		return fmt.Sprintf("%.0f", v)
	}
	if v >= 10 {
		return fmt.Sprintf("%.1f", v)
	}
	return fmt.Sprintf("%.2f", v)
}

func commaNits(v float64) string {
	whole := int64(math.Round(v))
	s := strconv.FormatInt(whole, 10)
	if whole < 0 {
		s = s[1:]
	}
	var out []byte
	for i, ch := range []byte(s) {
		if i > 0 && (len(s)-i)%3 == 0 {
			out = append(out, ',')
		}
		out = append(out, ch)
	}
	if whole < 0 {
		return "-" + string(out)
	}
	return string(out)
}
