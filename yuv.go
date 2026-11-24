package main

import (
	"errors"
	"image"
	"image/color"
	"math"
)

type ChromaSiting string

const (
	ChromaLeft   ChromaSiting = "left"
	ChromaCenter ChromaSiting = "center"
)

type YUV422 struct {
	Width  int
	Height int
	Siting ChromaSiting
	Y      []uint8
	Cb     []uint8
	Cr     []uint8
}

func EncodeYUV422(img image.Image, siting ChromaSiting) (*YUV422, error) {
	bounds := img.Bounds()
	width := bounds.Dx()
	height := bounds.Dy()
	if width%2 != 0 {
		return nil, errors.New("YUV 4:2:2 requires an even frame width")
	}

	frame := &YUV422{
		Width:  width,
		Height: height,
		Siting: siting,
		Y:      make([]uint8, width*height),
		Cb:     make([]uint8, (width/2)*height),
		Cr:     make([]uint8, (width/2)*height),
	}

	planeWidth := width / 2
	for y := 0; y < height; y++ {
		rowStart := y * width
		for x := 0; x < width; x++ {
			r8, g8, b8 := rgbAt(img, bounds.Min.X+x, bounds.Min.Y+y)
			frame.Y[rowStart+x] = clampByte(0.299*float64(r8) + 0.587*float64(g8) + 0.114*float64(b8))
		}
		for k := 0; k < planeWidth; k++ {
			cb, cr := sampleRGBChroma(img, bounds, siting, k*2, y)
			offset := y*planeWidth + k
			frame.Cb[offset] = cb
			frame.Cr[offset] = cr
		}
	}
	return frame, nil
}

func ConvertSiting(frame *YUV422, target ChromaSiting) (*YUV422, error) {
	if frame == nil {
		return nil, errors.New("nil YUV422 frame")
	}
	if frame.Siting != ChromaLeft && frame.Siting != ChromaCenter {
		return nil, errors.New("unsupported source chroma siting")
	}
	if target != ChromaLeft && target != ChromaCenter {
		return nil, errors.New("unsupported target chroma siting")
	}

	planeWidth := frame.Width / 2
	converted := &YUV422{
		Width:  frame.Width,
		Height: frame.Height,
		Siting: target,
		Y:      append([]uint8(nil), frame.Y...),
		Cb:     make([]uint8, len(frame.Cb)),
		Cr:     make([]uint8, len(frame.Cr)),
	}
	for y := 0; y < frame.Height; y++ {
		for k := 0; k < planeWidth; k++ {
			targetX := chromaX(target, k)
			offset := y*planeWidth + k
			converted.Cb[offset] = clampByte(sampleChroma(frame.Cb, frame.Width, y, targetX, frame.Siting))
			converted.Cr[offset] = clampByte(sampleChroma(frame.Cr, frame.Width, y, targetX, frame.Siting))
		}
	}
	return converted, nil
}

func Reconstruct(frame *YUV422, assumed ChromaSiting) *image.RGBA {
	output := image.NewRGBA(image.Rect(0, 0, frame.Width, frame.Height))
	for y := 0; y < frame.Height; y++ {
		for x := 0; x < frame.Width; x++ {
			offset := y*frame.Width + x
			cb := sampleChroma(frame.Cb, frame.Width, y, float64(x), assumed)
			cr := sampleChroma(frame.Cr, frame.Width, y, float64(x), assumed)
			r, g, b := yuvToRGB(float64(frame.Y[offset]), cb, cr)
			output.SetRGBA(x, y, color.RGBA{R: r, G: g, B: b, A: 255})
		}
	}
	return output
}

func chromaX(siting ChromaSiting, k int) float64 {
	if siting == ChromaCenter {
		return float64(2*k) + 0.5
	}
	return float64(2 * k)
}

func sampleChroma(plane []uint8, width int, row int, x float64, siting ChromaSiting) float64 {
	planeWidth := width / 2
	base := row * planeWidth
	if siting == ChromaLeft {
		k := int(math.Round(x / 2))
		k = clampInt(k, 0, planeWidth-1)
		return float64(plane[base+k])
	}

	position := (x - 0.5) / 2
	left := int(math.Floor(position))
	weight := position - float64(left)
	right := left + 1
	left = clampInt(left, 0, planeWidth-1)
	right = clampInt(right, 0, planeWidth-1)
	return float64(plane[base+left])*(1-weight) + float64(plane[base+right])*weight
}

func sampleRGBChroma(img image.Image, bounds image.Rectangle, siting ChromaSiting, evenX, y int) (uint8, uint8) {
	if siting == ChromaLeft {
		r, g, b := rgbAt(img, bounds.Min.X+evenX, bounds.Min.Y+y)
		return rgbChroma(r, g, b)
	}

	leftR, leftG, leftB := rgbAt(img, bounds.Min.X+evenX, bounds.Min.Y+y)
	rightX := evenX + 1
	if rightX >= bounds.Dx() {
		rightX = evenX
	}
	rightR, rightG, rightB := rgbAt(img, bounds.Min.X+rightX, bounds.Min.Y+y)
	cb1, cr1 := rgbChroma(leftR, leftG, leftB)
	cb2, cr2 := rgbChroma(rightR, rightG, rightB)
	return clampByte((float64(cb1) + float64(cb2)) / 2), clampByte((float64(cr1) + float64(cr2)) / 2)
}

func rgbAt(img image.Image, x, y int) (uint8, uint8, uint8) {
	c := img.At(x, y)
	red, green, blue, _ := c.RGBA()
	return uint8(red >> 8), uint8(green >> 8), uint8(blue >> 8)
}

func rgbChroma(r, g, b uint8) (uint8, uint8) {
	red := float64(r)
	green := float64(g)
	blue := float64(b)
	cb := -0.168736*red - 0.331264*green + 0.5*blue + 128
	cr := 0.5*red - 0.418688*green - 0.081312*blue + 128
	return clampByte(cb), clampByte(cr)
}

func yuvToRGB(y, cb, cr float64) (uint8, uint8, uint8) {
	cb -= 128
	cr -= 128
	r := y + 1.402*cr
	g := y - 0.344136*cb - 0.714136*cr
	b := y + 1.772*cb
	return clampByte(r), clampByte(g), clampByte(b)
}

func clampByte(value float64) uint8 {
	return uint8(clampInt(int(math.Round(value)), 0, 255))
}

func clampInt(value, low, high int) int {
	if value < low {
		return low
	}
	if value > high {
		return high
	}
	return value
}
