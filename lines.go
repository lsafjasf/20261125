package lut3d

import (
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"
)

var (
	floatTokenRE = regexp.MustCompile(`^[+-]?(?:[0-9]+(?:\.[0-9]*)?|\.[0-9]+)(?:[eE][+-]?[0-9]+)?$`)
	sizeTokenRE  = regexp.MustCompile(`^[0-9]+$`)
)

func stripComment(line string) string {
	inQuote := false
	escaped := false
	for i := 0; i < len(line); i++ {
		switch c := line[i]; {
		case escaped:
			escaped = false
		case inQuote && c == '\\':
			escaped = true
		case c == '"':
			inQuote = !inQuote
		case c == '#' && !inQuote:
			return strings.TrimSpace(line[:i])
		}
	}
	return strings.TrimSpace(line)
}

func parseFloatToken(token string) (float64, error) {
	if !floatTokenRE.MatchString(token) {
		return 0, fmt.Errorf("invalid decimal number %q", token)
	}
	v, err := strconv.ParseFloat(token, 64)
	if err != nil {
		return 0, err
	}
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return 0, fmt.Errorf("non-finite number %q", token)
	}
	return v, nil
}

func parseIntToken(token string) (int, error) {
	if !sizeTokenRE.MatchString(token) {
		return 0, fmt.Errorf("invalid positive integer %q", token)
	}
	v, err := strconv.ParseUint(token, 10, 31)
	if err != nil {
		return 0, err
	}
	return int(v), nil
}
