package main

import (
	"fmt"
	"html"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

type SVG struct {
	strings.Builder
	Width  int
	Height int
}

type Point struct {
	X, Y float64
}

type GradientStop struct {
	Offset float64
	Color  string
}

func (s *SVG) ff(format string, args ...any) {
	fmt.Fprintf(&s.Builder, format, args...)
}

func NewSVG(width, height int) *SVG {
	s := &SVG{Width: width, Height: height}
	s.WriteString(`<?xml version="1.0" encoding="UTF-8"?>` + "\n")
	s.WriteString(`<svg xmlns="http://www.w3.org/2000/svg" width="`)
	s.WriteString(itoa(width))
	s.WriteString(`" height="`)
	s.WriteString(itoa(height))
	s.WriteString(`" viewBox="0 0 `)
	s.WriteString(itoa(width))
	s.WriteByte(' ')
	s.WriteString(itoa(height))
	s.WriteString(`" font-family="Inter, 'Noto Sans CJK SC', 'Microsoft YaHei', sans-serif">`)
	return s
}

func itoa(v int) string {
	return strconv.Itoa(v)
}

func (s *SVG) Close() {
	s.WriteString("</svg>\n")
}

func (s *SVG) Save(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(s.String()), 0o644)
}

func (s *SVG) Rect(x, y, w, h float64, fill string, extra ...string) {
	s.WriteString(`<rect x="`)
	s.ff("%.2f", x)
	s.WriteString(`" y="`)
	s.ff("%.2f", y)
	s.WriteString(`" width="`)
	s.ff("%.2f", w)
	s.WriteString(`" height="`)
	s.ff("%.2f", h)
	s.WriteString(`" fill="`)
	s.WriteString(fill)
	s.WriteByte('"')
	for _, attr := range extra {
		s.WriteByte(' ')
		s.WriteString(attr)
	}
	s.WriteString("/>\n")
}

func (s *SVG) Text(x, y float64, size int, fill, anchor, weight, text string) {
	s.WriteString(`<text x="`)
	s.ff("%.2f", x)
	s.WriteString(`" y="`)
	s.ff("%.2f", y)
	s.WriteString(`" font-size="`)
	s.WriteString(itoa(size))
	s.WriteString(`" fill="`)
	s.WriteString(fill)
	if anchor != "" {
		s.WriteString(`" text-anchor="`)
		s.WriteString(anchor)
	}
	if weight != "" {
		s.WriteString(`" font-weight="`)
		s.WriteString(weight)
	}
	s.WriteString(`">`)
	s.WriteString(html.EscapeString(text))
	s.WriteString("</text>\n")
}

func (s *SVG) Line(x1, y1, x2, y2 float64, stroke string, width float64, dash string) {
	s.WriteString(`<line x1="`)
	s.ff("%.2f", x1)
	s.WriteString(`" y1="`)
	s.ff("%.2f", y1)
	s.WriteString(`" x2="`)
	s.ff("%.2f", x2)
	s.WriteString(`" y2="`)
	s.ff("%.2f", y2)
	s.WriteString(`" stroke="`)
	s.WriteString(stroke)
	s.WriteString(`" stroke-width="`)
	s.ff("%.2f", width)
	if dash != "" {
		s.WriteString(`" stroke-dasharray="`)
		s.WriteString(dash)
	}
	s.WriteString("\"/>\n")
}

func (s *SVG) Circle(x, y, r float64, fill, stroke string, width float64) {
	s.WriteString(`<circle cx="`)
	s.ff("%.2f", x)
	s.WriteString(`" cy="`)
	s.ff("%.2f", y)
	s.WriteString(`" r="`)
	s.ff("%.2f", r)
	s.WriteString(`" fill="`)
	s.WriteString(fill)
	s.WriteString(`" stroke="`)
	s.WriteString(stroke)
	s.WriteString(`" stroke-width="`)
	s.ff("%.2f", width)
	s.WriteString("\"/>\n")
}

func (s *SVG) Path(points []Point, stroke string, width float64, dash string) {
	if len(points) < 2 {
		return
	}
	s.WriteString(`<path d="M`)
	for i, p := range points {
		if i > 0 {
			s.WriteByte('L')
		}
		s.ff("%.2f %.2f ", p.X, p.Y)
	}
	s.WriteString(`" fill="none" stroke="`)
	s.WriteString(stroke)
	s.WriteString(`" stroke-width="`)
	s.ff("%.2f", width)
	s.WriteString(`" stroke-linecap="round" stroke-linejoin="round"`)
	if dash != "" {
		s.WriteString(` stroke-dasharray="`)
		s.WriteString(dash)
		s.WriteByte('"')
	}
	s.WriteString("/>\n")
}

func (s *SVG) LinearGradient(id string, horizontal bool, stops []GradientStop) {
	s.WriteString(`<defs><linearGradient id="`)
	s.WriteString(id)
	s.WriteString(`" `)
	if horizontal {
		s.WriteString(`x1="0" y1="0" x2="1" y2="0"`)
	} else {
		s.WriteString(`x1="0" y1="0" x2="0" y2="1"`)
	}
	s.WriteString(`>`)
	for _, stop := range stops {
		s.ff(`<stop offset="%.3f" stop-color="%s"/>`, stop.Offset, stop.Color)
	}
	s.WriteString(`</linearGradient></defs>` + "\n")
}
