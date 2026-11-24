// Package strip defines the graphic-subtitle strip model and manifest loading.
package strip

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// Manifest is the root of the graphic-strip export the team hands over.
type Manifest struct {
	Video  Video   `json:"video"`
	Strips []Strip `json:"strips"`
}

// Video describes the canvas the strip coordinates refer to.
type Video struct {
	Width  int `json:"width"`
	Height int `json:"height"`
}

// Strip is one graphic subtitle event (one bitmap on the timeline).
type Strip struct {
	ID         string   `json:"id"`
	Start      string   `json:"start"` // HH:MM:SS.mmm
	End        string   `json:"end"`
	Text       string   `json:"text"`  // empty means image-only, not yet transcribed
	Image      string   `json:"image"` // original bitmap, relative to manifest dir
	Box        Box      `json:"box"`
	Anchor     string   `json:"anchor"` // e.g. bottom-center, top-left
	Rotation   float64  `json:"rotationDeg"`
	Fill       string   `json:"fill"` // #RRGGBB or #RRGGBBAA
	FontSizePx int      `json:"fontSizePx"`
	Outline    *Outline `json:"outline"`
	Shadow     *Shadow  `json:"shadow"`
	FadeInMs   int      `json:"fadeInMs"`
	FadeOutMs  int      `json:"fadeOutMs"`
	Align      string   `json:"align"` // left, center, right
}

// Box is the pixel rectangle of the strip on the video canvas.
type Box struct {
	X int `json:"x"`
	Y int `json:"y"`
	W int `json:"w"`
	H int `json:"h"`
}

// Outline is a stroke drawn around glyph edges.
type Outline struct {
	WidthPx float64 `json:"widthPx"`
	Color   string  `json:"color"`
}

// Shadow is a drop shadow behind the glyphs.
type Shadow struct {
	OffsetXPx float64 `json:"offsetXPx"`
	OffsetYPx float64 `json:"offsetYPx"`
	Color     string  `json:"color"`
}

// Load reads and validates a manifest file.
func Load(path string) (*Manifest, string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, "", err
	}
	var m Manifest
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, "", fmt.Errorf("parse manifest: %w", err)
	}
	if m.Video.Width <= 0 || m.Video.Height <= 0 {
		return nil, "", fmt.Errorf("manifest video size must be positive, got %+v", m.Video)
	}
	for i := range m.Strips {
		s := &m.Strips[i]
		if s.ID == "" {
			return nil, "", fmt.Errorf("strips[%d]: empty id", i)
		}
		if _, err := ParseTimestamp(s.Start); err != nil {
			return nil, "", fmt.Errorf("strip %s: bad start: %w", s.ID, err)
		}
		if _, err := ParseTimestamp(s.End); err != nil {
			return nil, "", fmt.Errorf("strip %s: bad end: %w", s.ID, err)
		}
	}
	return &m, filepath.Dir(path), nil
}

// ParseTimestamp parses HH:MM:SS.mmm (also accepts MM:SS.mmm).
func ParseTimestamp(s string) (time.Duration, error) {
	parts := strings.Split(s, ":")
	if len(parts) < 2 || len(parts) > 3 {
		return 0, fmt.Errorf("invalid timestamp %q", s)
	}
	if len(parts) == 2 {
		parts = append([]string{"0"}, parts...)
	}
	h, err := strconv.Atoi(parts[0])
	if err != nil {
		return 0, fmt.Errorf("invalid hours in %q", s)
	}
	min, err := strconv.Atoi(parts[1])
	if err != nil {
		return 0, fmt.Errorf("invalid minutes in %q", s)
	}
	sec, err := strconv.ParseFloat(parts[2], 64)
	if err != nil {
		return 0, fmt.Errorf("invalid seconds in %q", s)
	}
	d := time.Duration(h)*time.Hour + time.Duration(min)*time.Minute +
		time.Duration(sec*float64(time.Second))
	return d, nil
}

// StartEnd returns the parsed cue interval.
func (s *Strip) StartEnd() (time.Duration, time.Duration, error) {
	start, err := ParseTimestamp(s.Start)
	if err != nil {
		return 0, 0, err
	}
	end, err := ParseTimestamp(s.End)
	if err != nil {
		return 0, 0, err
	}
	return start, end, nil
}
