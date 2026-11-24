// Command mkfixture synthesizes a deterministic YUV420p test sequence that
// contains smooth dark gradients, sharp text edges, posterized tones and a
// fast-moving block, so the detector can be validated end to end.
package main

import (
	"bufio"
	"flag"
	"fmt"
	"image"
	"image/png"
	"os"
	"path/filepath"
)

func main() {
	outYUV := flag.String("yuv", "", "output raw I420 file")
	outPNG := flag.String("pngdir", "", "output PNG directory (reference frames)")
	w := flag.Int("width", 320, "width")
	h := flag.Int("height", 256, "height")
	frames := flag.Int("frames", 24, "frame count")
	flag.Parse()

	var yuv *bufio.Writer
	if *outYUV != "" {
		f, err := os.Create(*outYUV)
		if err != nil {
			panic(err)
		}
		defer f.Close()
		yuv = bufio.NewWriterSize(f, 1<<20)
	}

	for fi := 0; fi < *frames; fi++ {
		y := renderFrame(*w, *h, fi, *frames)
		if yuv != nil {
			if _, err := yuv.Write(y); err != nil {
				panic(err)
			}
			chroma := make([]byte, (*w**h)/2)
			for i := range chroma {
				chroma[i] = 128
			}
			if _, err := yuv.Write(chroma); err != nil {
				panic(err)
			}
		}
		if *outPNG != "" {
			if err := os.MkdirAll(*outPNG, 0o755); err != nil {
				panic(err)
			}
			img := image.NewGray(image.Rect(0, 0, *w, *h))
			copy(img.Pix, y)
			name := filepath.Join(*outPNG, fmt.Sprintf("f_%04d.png", fi))
			f, err := os.Create(name)
			if err != nil {
				panic(err)
			}
			if err := png.Encode(f, img); err != nil {
				panic(err)
			}
			f.Close()
		}
	}
	if yuv != nil {
		yuv.Flush()
	}
}

func renderFrame(w, h, fi, n int) []byte {
	y := make([]byte, w*h)
	for row := 0; row < h; row++ {
		for col := 0; col < w; col++ {
			// Smooth dark vertical gradient: luma 6..70, ideal banding trigger.
			v := 6 + (row*64)/h
			y[row*w+col] = byte(v)
		}
	}

	// Bright flat panel in the upper-right with sharp text on it.
	panelX, panelY, panelW, panelH := w*55/100, 8, w*42/100, h*34/100
	fillRect(y, w, panelX, panelY, panelW, panelH, 205)
	drawText(y, w, panelX+8, panelY+10, "ARTIFACT")
	drawText(y, w, panelX+8, panelY+26, "DIAG 2026")

	// Strong posterization region (8 hard levels): tone/grade damage signature.
	px, py, pw, ph := 8, h*62/100, w*42/100, h*30/100
	for row := py; row < py+ph; row++ {
		step := ((row - py) * 8) / ph
		fillRect(y, w, px, row, pw, 1, byte(20+step*7))
	}

	// Fast-moving bright square: large position change between frames.
	move := (fi * 14) % (w - 24)
	fillRect(y, w, move, h*46/100, 18, 18, 235)

	// Slow-moving square for contrast (should not be flagged as motion).
	slow := fi
	fillRect(y, w, w-30-slow%10, h*85/100, 12, 12, 180)
	return y
}

func fillRect(y []byte, w, x0, y0, rw, rh int, v byte) {
	for row := y0; row < y0+rh && row < len(y)/w; row++ {
		for col := x0; col < x0+rw && col < w; col++ {
			if row >= 0 && col >= 0 {
				y[row*w+col] = v
			}
		}
	}
}

// Minimal 5x7 bitmap font, uppercase letters and digits only.
var glyphs = map[byte][]string{
	'A': {"01110", "10001", "10001", "11111", "10001", "10001", "10001"},
	'C': {"01111", "10000", "10000", "10000", "10000", "10000", "01111"},
	'D': {"11110", "10001", "10001", "10001", "10001", "10001", "11110"},
	'F': {"11111", "10000", "10000", "11110", "10000", "10000", "10000"},
	'G': {"01111", "10000", "10000", "10111", "10001", "10001", "01111"},
	'I': {"11111", "00100", "00100", "00100", "00100", "00100", "11111"},
	'R': {"11110", "10001", "10001", "11110", "10100", "10010", "10001"},
	'T': {"11111", "00100", "00100", "00100", "00100", "00100", "00100"},
	'2': {"01110", "10001", "00001", "00010", "00100", "01000", "11111"},
	'0': {"01110", "10001", "10011", "10101", "11001", "10001", "01110"},
	'6': {"00110", "01000", "10000", "11110", "10001", "10001", "01110"},
	' ': {"00000", "00000", "00000", "00000", "00000", "00000", "00000"},
}

func drawText(y []byte, w, x, y0 int, text string) {
	cx := x
	for i := 0; i < len(text); i++ {
		g, ok := glyphs[text[i]]
		if !ok {
			g = glyphs[' ']
		}
		for row := 0; row < 7; row++ {
			for col := 0; col < 5; col++ {
				if g[row][col] == '1' {
					px, py := cx+col, y0+row
					if py >= 0 && py < len(y)/w && px >= 0 && px < w {
						y[py*w+px] = 25
					}
				}
			}
		}
		cx += 6
	}
}
