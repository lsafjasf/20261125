package audit

import "fmt"

func blackLevel(s SampleStats, r LegalRange) BlackLevel {
	floor := s.Percent.P001
	shadow := s.Percent.P05
	pedestal := floor - r.Black
	if pedestal < 0 {
		pedestal = 0
	}
	span := r.White - r.Black
	percent := 0.0
	if span > 0 {
		percent = float64(pedestal) / float64(span) * 100
	}
	severity := "none"
	switch {
	case pedestal <= 1:
		severity = "none"
	case pedestal <= 3:
		severity = "soft"
	case pedestal <= 8:
		severity = "moderate"
	default:
		severity = "strong"
	}

	lowKeyThreshold := r.Black + span/4
	deepShadowThreshold := r.Black + span/5
	style := "ordinary-shadow-placement"
	if s.Percent.P50 <= lowKeyThreshold && shadow <= deepShadowThreshold {
		style = "low-key-or-stylized-dark-compatible"
	}

	reading := "nominal-black"
	alternatives := []string(nil)
	if pedestal > 1 {
		reading = "lifted-black"
		alternatives = append(alternatives, "stylized dark shadows with intentional black avoidance")
		if style == "low-key-or-stylized-dark-compatible" {
			alternatives = append(alternatives, "true technical pedestal applied uniformly to the low end")
		}
	}
	if pedestal <= 1 && style == "low-key-or-stylized-dark-compatible" {
		reading = "nominal-black-with-low-key-shadows"
	}
	return BlackLevel{
		Anchor:          r.Black,
		ObservedFloor:   floor,
		TypicalShadow:   shadow,
		PedestalCodes:   pedestal,
		PedestalPercent: percent,
		Severity:        severity,
		Reading:         reading,
		StyleReading:    style,
		Alternatives:    alternatives,
	}
}

func (b BlackLevel) Compact() string {
	return fmt.Sprintf("anchor=%d floor=%d pedestal=%+d codes (%.2f%%, %s), %s", b.Anchor, b.ObservedFloor, b.PedestalCodes, b.PedestalPercent, b.Severity, b.Reading)
}
