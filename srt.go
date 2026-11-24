package main

import (
	"fmt"
	"math/big"
	"strconv"
	"strings"
)

type Cue struct {
	Index int
	Start *big.Rat
	End   *big.Rat
	Text  string
}

func parseSRT(data []byte) ([]*Cue, error) {
	content := strings.TrimPrefix(string(data), "\uFEFF")
	content = strings.ReplaceAll(content, "\r\n", "\n")
	content = strings.ReplaceAll(content, "\r", "\n")

	var cues []*Cue
	for blockNumber, block := range strings.Split(content, "\n\n") {
		block = strings.TrimSpace(block)
		if block == "" {
			continue
		}

		lines := strings.Split(block, "\n")
		pos := 0
		index := blockNumber + 1
		if len(lines) > 0 && isInteger(lines[0]) {
			parsedIndex, err := strconv.Atoi(strings.TrimSpace(lines[0]))
			if err != nil {
				return nil, fmt.Errorf("block %d: invalid cue index: %w", blockNumber+1, err)
			}
			index = parsedIndex
			pos = 1
		}
		if pos >= len(lines) {
			return nil, fmt.Errorf("block %d: missing timestamp line", blockNumber+1)
		}

		start, end, err := parseTimestampLine(lines[pos])
		if err != nil {
			return nil, fmt.Errorf("block %d: %w", blockNumber+1, err)
		}
		pos++
		if pos >= len(lines) || strings.TrimSpace(strings.Join(lines[pos:], "\n")) == "" {
			return nil, fmt.Errorf("cue %d: missing subtitle text", index)
		}

		cues = append(cues, &Cue{
			Index: index,
			Start: start,
			End:   end,
			Text:  strings.TrimSpace(strings.Join(lines[pos:], "\n")),
		})
	}
	if len(cues) == 0 {
		return nil, fmt.Errorf("no SRT cues found")
	}
	return cues, nil
}

func formatSRT(cues []*Cue) (string, error) {
	var builder strings.Builder
	for i, cue := range cues {
		start, err := formatRatMilliseconds(cue.Start)
		if err != nil {
			return "", fmt.Errorf("cue %d start: %w", cue.Index, err)
		}
		end, err := formatRatMilliseconds(cue.End)
		if err != nil {
			return "", fmt.Errorf("cue %d end: %w", cue.Index, err)
		}
		if i > 0 {
			builder.WriteString("\n")
		}
		builder.WriteString(strconv.Itoa(cue.Index))
		builder.WriteString("\n")
		builder.WriteString(start)
		builder.WriteString(" --> ")
		builder.WriteString(end)
		builder.WriteString("\n")
		builder.WriteString(cue.Text)
		builder.WriteString("\n")
	}
	return builder.String(), nil
}

func parseTimestampLine(line string) (*big.Rat, *big.Rat, error) {
	parts := strings.Split(line, " --> ")
	if len(parts) != 2 {
		return nil, nil, fmt.Errorf("timestamp line must use Start --> End")
	}
	start, err := parseTimestamp(strings.TrimSpace(parts[0]))
	if err != nil {
		return nil, nil, fmt.Errorf("start: %w", err)
	}
	end, err := parseTimestamp(strings.TrimSpace(parts[1]))
	if err != nil {
		return nil, nil, fmt.Errorf("end: %w", err)
	}
	return start, end, nil
}

func parseTimestamp(value string) (*big.Rat, error) {
	comma := strings.Split(value, ",")
	if len(comma) != 2 {
		return nil, fmt.Errorf("timestamp %q must use HH:MM:SS,mmm", value)
	}
	clockParts := strings.Split(comma[0], ":")
	if len(clockParts) != 3 {
		return nil, fmt.Errorf("timestamp %q must use HH:MM:SS", value)
	}

	hours, err := parseNonNegativeBigInt(clockParts[0], "hours")
	if err != nil {
		return nil, err
	}
	minutes, err := parseNonNegativeBigInt(clockParts[1], "minutes")
	if err != nil {
		return nil, err
	}
	seconds, err := parseNonNegativeBigInt(clockParts[2], "seconds")
	if err != nil {
		return nil, err
	}
	milliseconds, err := parseFixedWidthBigInt(comma[1], 3, "milliseconds")
	if err != nil {
		return nil, err
	}
	if minutes.Cmp(big.NewInt(60)) >= 0 || seconds.Cmp(big.NewInt(60)) >= 0 {
		return nil, fmt.Errorf("timestamp %q has minutes or seconds outside 0..59", value)
	}

	total := new(big.Int).Set(hours)
	total.Mul(total, big.NewInt(60))
	total.Add(total, minutes)
	total.Mul(total, big.NewInt(60))
	total.Add(total, seconds)
	total.Mul(total, big.NewInt(1000))
	total.Add(total, milliseconds)
	return new(big.Rat).SetInt(total), nil
}

func formatRatMilliseconds(value *big.Rat) (string, error) {
	if value == nil {
		return "", fmt.Errorf("missing timestamp")
	}
	if value.Sign() < 0 {
		return "", fmt.Errorf("timestamp cannot be negative")
	}
	if value.Denom().Cmp(big.NewInt(1)) != 0 {
		return "", fmt.Errorf("timestamp is not quantized to a millisecond: %s", value.RatString())
	}

	remaining := new(big.Int).Set(value.Num())
	milliseconds := new(big.Int)
	remaining.DivMod(remaining, big.NewInt(1000), milliseconds)
	seconds := new(big.Int)
	remaining.DivMod(remaining, big.NewInt(60), seconds)
	minutes := new(big.Int)
	remaining.DivMod(remaining, big.NewInt(60), minutes)

	return fmt.Sprintf("%02s:%02s:%02s,%03s", remaining.String(), minutes.String(), seconds.String(), milliseconds.String()), nil
}

func parseNonNegativeBigInt(value, name string) (*big.Int, error) {
	number, ok := new(big.Int).SetString(value, 10)
	if !ok || number.Sign() < 0 {
		return nil, fmt.Errorf("invalid %s %q", name, value)
	}
	return number, nil
}

func parseFixedWidthBigInt(value string, width int, name string) (*big.Int, error) {
	number, err := parseNonNegativeBigInt(value, name)
	if err != nil || len(value) != width {
		return nil, fmt.Errorf("%s must be exactly %d digits", name, width)
	}
	return number, nil
}

func isInteger(value string) bool {
	for _, r := range strings.TrimSpace(value) {
		if r < '0' || r > '9' {
			return false
		}
	}
	return strings.TrimSpace(value) != ""
}
