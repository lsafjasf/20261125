package main

import (
	"errors"
	"fmt"
	"strings"
)

const (
	speakerFL   = "FL"
	speakerFR   = "FR"
	speakerFC   = "FC"
	speakerLFE  = "LFE"
	speakerBL   = "BL"
	speakerBR   = "BR"
	speakerFLC  = "FLC"
	speakerFRC  = "FRC"
	speakerBC   = "BC"
	speakerSL   = "SL"
	speakerSR   = "SR"
	speakerTC   = "TC"
	speakerTFC  = "TFC"
	speakerTFL  = "TFL"
	speakerTFC2 = "TFC2"
	speakerTFR  = "TFR"
	speakerTBL  = "TBL"
	speakerTBR  = "TBR"
)

var speakerOrder = []struct {
	bit  uint32
	name string
}{
	{1 << 0, speakerFL},
	{1 << 1, speakerFR},
	{1 << 2, speakerFC},
	{1 << 3, speakerLFE},
	{1 << 4, speakerBL},
	{1 << 5, speakerBR},
	{1 << 6, speakerFLC},
	{1 << 7, speakerFRC},
	{1 << 8, speakerBC},
	{1 << 9, speakerSL},
	{1 << 10, speakerSR},
	{1 << 11, speakerTC},
	{1 << 12, speakerTFC},
	{1 << 13, speakerTFL},
	{1 << 14, speakerTFC2},
	{1 << 15, speakerTFR},
	{1 << 16, speakerTBL},
	{1 << 17, speakerTBR},
}

func maskNames(mask uint32) []string {
	names := make([]string, 0)
	for _, item := range speakerOrder {
		if mask&item.bit != 0 {
			names = append(names, item.name)
		}
	}
	for bit := uint(18); bit < 32; bit++ {
		if mask&(1<<bit) != 0 {
			names = append(names, fmt.Sprintf("BIT%d", bit))
		}
	}
	return names
}

func numberedNames(count int) []string {
	names := make([]string, count)
	for i := range names {
		names[i] = fmt.Sprintf("CH%d", i+1)
	}
	return names
}

func parseMapping(text string, channels int) ([]int, error) {
	parts := strings.FieldsFunc(text, func(r rune) bool {
		return r == ',' || r == ' ' || r == '\t'
	})
	if len(parts) != channels {
		return nil, fmt.Errorf("mapping needs %d source positions, got %d", channels, len(parts))
	}
	mapping := make([]int, channels)
	seen := make(map[int]bool, channels)
	for i, part := range parts {
		var value int
		if _, err := fmt.Sscanf(part, "%d", &value); err != nil {
			return nil, fmt.Errorf("invalid mapping position %q", part)
		}
		if value < 1 || value > channels {
			return nil, fmt.Errorf("mapping position %d is outside 1..%d", value, channels)
		}
		mapping[i] = value - 1
		if seen[mapping[i]] {
			return nil, fmt.Errorf("source position %d is used more than once", value)
		}
		seen[mapping[i]] = true
	}
	return mapping, nil
}

func parseNames(text string, count int) ([]string, error) {
	names := strings.FieldsFunc(text, func(r rune) bool {
		return r == ',' || r == ' ' || r == '\t'
	})
	if len(names) != count {
		return nil, fmt.Errorf("declaration needs %d names, got %d", count, len(names))
	}
	seen := map[string]bool{}
	for _, name := range names {
		if name == "" {
			return nil, errors.New("empty channel name")
		}
		if seen[name] {
			return nil, fmt.Errorf("channel name %q is repeated", name)
		}
		seen[name] = true
	}
	return names, nil
}

func identityMapping(count int) []int {
	mapping := make([]int, count)
	for i := range mapping {
		mapping[i] = i
	}
	return mapping
}

func isIdentity(mapping []int) bool {
	for i, value := range mapping {
		if value != i {
			return false
		}
	}
	return true
}
