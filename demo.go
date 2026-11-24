package main

import (
	"fmt"
	"image"
	"image/color"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func MakeDemoFixture(path, ffmpegBin string) (string, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", err
	}
	referencePath := strings.TrimSuffix(path, filepath.Ext(path)) + ".reference.png"
	upright := DrawOrientationMarker()
	if err := savePNG(referencePath, upright); err != nil {
		return "", err
	}

	baked := rotateTurns(upright, 1)
	tmpDir, err := os.MkdirTemp("", "rotationaudit-demo-")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(tmpDir)
	bakedPath := filepath.Join(tmpDir, "baked-clockwise-once.png")
	if err := savePNG(bakedPath, baked); err != nil {
		return "", err
	}

	cmd := exec.Command(ffmpegBin,
		"-hide_banner", "-loglevel", "error", "-y",
		"-loop", "1", "-framerate", "1", "-i", bakedPath,
		"-frames:v", "1", "-c:v", "libx264", "-pix_fmt", "yuv420p",
		"-movflags", "+faststart", path)
	if out, err := cmd.CombinedOutput(); err != nil {
		return "", fmt.Errorf("encode demo MP4: %w (%s)", err, string(out))
	}

	// Coded pixels are 180x320. The matrix turns clockwise once, producing
	// 320x180. tkhd also claims 320x180, so the axis swap is geometrically
	// consistent; semantic inspection reveals the second rotation.
	if err := PatchTKHDTransform(path, 1, 320, 180); err != nil {
		return "", fmt.Errorf("patch demo display matrix: %w", err)
	}
	return referencePath, nil
}

func DrawOrientationMarker() *image.RGBA {
	const w = 320
	const h = 180
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	fillRect(img, img.Bounds(), color.RGBA{R: 255, G: 255, B: 255, A: 255})

	red := color.RGBA{R: 220, G: 38, B: 38, A: 255}
	green := color.RGBA{R: 22, G: 163, B: 74, A: 255}
	blue := color.RGBA{R: 37, G: 99, B: 235, A: 255}
	black := color.RGBA{R: 17, G: 24, B: 39, A: 255}
	yellow := color.RGBA{R: 250, G: 204, B: 21, A: 255}

	fillRect(img, image.Rect(0, 0, w, 34), red)
	fillRect(img, image.Rect(0, h-12, w, h), yellow)
	fillRect(img, image.Rect(248, 122, 306, 168), blue)
	fillTriangle(img, 176, 168, 222, 82, 268, 168, green)

	// Block letter F is intentionally asymmetric and remains readable after
	// only 90/180/270-degree rotations.
	fillRect(img, image.Rect(42, 46, 82, 160), black)
	fillRect(img, image.Rect(42, 46, 202, 84), black)
	fillRect(img, image.Rect(42, 96, 168, 130), black)

	// Small corner ticks make cropping/scaling easier to spot.
	fillRect(img, image.Rect(8, 44, 24, 52), black)
	fillRect(img, image.Rect(8, 44, 16, 78), black)
	fillRect(img, image.Rect(296, 44, 312, 52), black)
	fillRect(img, image.Rect(304, 44, 312, 78), black)
	return img
}

func fillTriangle(img *image.RGBA, x1, y1, x2, y2, x3, y3 int, c color.RGBA) {
	minY := minInt(y1, minInt(y2, y3))
	maxY := maxInt(y1, maxInt(y2, y3))
	minX := minInt(x1, minInt(x2, x3))
	maxX := maxInt(x1, maxInt(x2, x3))
	for y := minY; y <= maxY; y++ {
		for x := minX; x <= maxX; x++ {
			if pointInTriangle(float64(x)+0.5, float64(y)+0.5,
				float64(x1), float64(y1), float64(x2), float64(y2), float64(x3), float64(y3)) {
				img.SetRGBA(x, y, c)
			}
		}
	}
}

func pointInTriangle(px, py, ax, ay, bx, by, cx, cy float64) bool {
	d1 := sign(px, py, ax, ay, bx, by)
	d2 := sign(px, py, bx, by, cx, cy)
	d3 := sign(px, py, cx, cy, ax, ay)
	hasNeg := d1 < 0 || d2 < 0 || d3 < 0
	hasPos := d1 > 0 || d2 > 0 || d3 > 0
	return !(hasNeg && hasPos)
}

func sign(px, py, x1, y1, x2, y2 float64) float64 {
	return (px-x2)*(y1-y2) - (x1-x2)*(py-y2)
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
