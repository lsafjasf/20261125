// Package sample generates a demo manifest plus placeholder strip bitmaps
// so the audit pipeline can be exercised end to end.
package sample

import (
	"encoding/json"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"os"
	"path/filepath"

	"subaudit/internal/strip"
)

// Write creates dir/images/*.png and dir/strips.json.
func Write(dir string) error {
	imgDir := filepath.Join(dir, "images")
	if err := os.MkdirAll(imgDir, 0o755); err != nil {
		return err
	}
	m := strip.Manifest{
		Video: strip.Video{Width: 1920, Height: 1080},
		Strips: []strip.Strip{
			{ID: "0001", Start: "00:00:01.000", End: "00:00:04.000",
				Text: "你好，世界", Image: "images/0001.png",
				Box:    strip.Box{X: 660, Y: 980, W: 600, H: 60},
				Anchor: "bottom-center", Fill: "#FFFFFF", FontSizePx: 42, Align: "center"},
			{ID: "0002", Start: "00:00:05.000", End: "00:00:08.500",
				Text: "带描边的台词", Image: "images/0002.png",
				Box:    strip.Box{X: 660, Y: 980, W: 600, H: 60},
				Anchor: "bottom-center", Fill: "#FFFFFF", FontSizePx: 42, Align: "center",
				Outline: &strip.Outline{WidthPx: 2.5, Color: "#000000"}},
			{ID: "0003", Start: "00:00:09.000", End: "00:00:12.000",
				Text: "淡入淡出的台词", Image: "images/0003.png",
				Box:    strip.Box{X: 660, Y: 980, W: 600, H: 60},
				Anchor: "bottom-center", Fill: "#FFFFFF", FontSizePx: 42, Align: "center",
				FadeInMs: 400, FadeOutMs: 600},
			{ID: "0004", Start: "00:00:13.000", End: "00:00:16.000",
				Text: "左上角标题", Image: "images/0004.png",
				Box:    strip.Box{X: 80, Y: 60, W: 500, H: 70},
				Anchor: "top-left", Fill: "#FFE080", FontSizePx: 54, Align: "left"},
			{ID: "0005", Start: "00:00:13.500", End: "00:00:15.500",
				Text: "同时出现的底部台词", Image: "images/0005.png",
				Box:    strip.Box{X: 660, Y: 980, W: 600, H: 60},
				Anchor: "bottom-center", Fill: "#FFFFFF", FontSizePx: 42, Align: "center"},
			{ID: "0006", Start: "00:00:17.000", End: "00:00:20.000",
				Text: "竖排旋转文字", Image: "images/0006.png",
				Box:    strip.Box{X: 1700, Y: 300, W: 80, H: 500},
				Anchor: "top-right", Fill: "#FFFFFF", FontSizePx: 40,
				Rotation: 90, Align: "center"},
			{ID: "0007", Start: "00:00:21.000", End: "00:00:24.000",
				Text: "", Image: "images/0007.png",
				Box:    strip.Box{X: 660, Y: 980, W: 600, H: 60},
				Anchor: "bottom-center"},
		},
	}
	for _, s := range m.Strips {
		if err := drawPlaceholder(filepath.Join(dir, s.Image), s); err != nil {
			return err
		}
	}
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "strips.json"), data, 0o644)
}

// drawPlaceholder renders a simple bitmap standing in for the real strip:
// a dark box with a border whose color encodes the strip's outline/fill.
func drawPlaceholder(path string, s strip.Strip) error {
	w, h := s.Box.W, s.Box.H
	if w <= 0 {
		w = 200
	}
	if h <= 0 {
		h = 60
	}
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	draw.Draw(img, img.Bounds(), &image.Uniform{color.RGBA{30, 30, 30, 255}}, image.Point{}, draw.Src)
	border := color.RGBA{255, 255, 255, 255}
	if s.Outline != nil {
		border = color.RGBA{255, 80, 80, 255}
	}
	bw := 4
	for x := 0; x < w; x++ {
		for y := 0; y < bw; y++ {
			img.Set(x, y, border)
			img.Set(x, h-1-y, border)
		}
	}
	for y := 0; y < h; y++ {
		for x := 0; x < bw; x++ {
			img.Set(x, y, border)
			img.Set(w-1-x, y, border)
		}
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return png.Encode(f, img)
}
