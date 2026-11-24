package wav

import (
	"fmt"
	"strings"
)

var speakerNames = []struct {
	bit  uint32
	name string
}{
	{0x0001, "FL"}, {0x0002, "FR"}, {0x0004, "FC"}, {0x0008, "LFE"},
	{0x0010, "BL"}, {0x0020, "BR"}, {0x0040, "FLC"}, {0x0080, "FRC"},
	{0x0100, "BC"}, {0x0200, "SL"}, {0x0400, "SR"}, {0x0800, "TFL"},
	{0x1000, "TFC"}, {0x2000, "TFR"}, {0x4000, "TBL"}, {0x8000, "TBC"},
	{0x10000, "TBR"},
}

// LayoutFromMask returns channels in WAVE speaker-bit order.
func LayoutFromMask(mask uint32, channels int) []string {
	out := make([]string, 0, channels)
	for _, speaker := range speakerNames {
		if mask&speaker.bit != 0 {
			out = append(out, speaker.name)
		}
	}
	if len(out) != channels {
		return nil
	}
	return out
}

func defaultLayout(channels int) []string {
	switch channels {
	case 1:
		return []string{"FC"}
	case 2:
		return []string{"FL", "FR"}
	case 6:
		return []string{"FL", "FR", "FC", "LFE", "BL", "BR"}
	}
	out := make([]string, channels)
	for i := range out {
		out[i] = fmt.Sprintf("CH%d", i+1)
	}
	return out
}

// ParseLayout accepts common aliases and preserves the given channel order.
func ParseLayout(spec string, channels int) ([]string, error) {
	parts := strings.Split(spec, ",")
	if len(parts) != channels {
		return nil, fmt.Errorf("layout has %d names, file has %d channels", len(parts), channels)
	}
	out := make([]string, channels)
	seen := make(map[string]bool, channels)
	for i, part := range parts {
		name := normalizeName(strings.TrimSpace(part))
		if name == "" {
			return nil, fmt.Errorf("layout channel %d is empty", i+1)
		}
		if seen[name] {
			return nil, fmt.Errorf("duplicate channel name %q", name)
		}
		seen[name] = true
		out[i] = name
	}
	return out, nil
}

func normalizeName(name string) string {
	name = strings.ToUpper(strings.ReplaceAll(strings.TrimSpace(name), " ", ""))
	switch name {
	case "L", "LEFT":
		return "FL"
	case "R", "RIGHT":
		return "FR"
	case "C", "CENTER", "CENTRE":
		return "FC"
	case "LS", "SL", "SURROUND-LEFT", "SURROUNDLEFT":
		return "BL"
	case "RS", "SR", "SURROUND-RIGHT", "SURROUNDRIGHT":
		return "BR"
	}
	return name
}

func column(value string, width int) string {
	if len(value) >= width {
		return value
	}
	return value + strings.Repeat(" ", width-len(value))
}
