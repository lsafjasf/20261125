package analyze

import (
	"fmt"
	"image"
	"image/color"
	_ "image/png" // register PNG decoder
	"os"
)

// LoadFrame decodes a PNG file into a Frame of PQ code values.
//
// bitDepth selects the signal range:
//   - 0  -> auto-detect (8 for 8-bit container, 16 otherwise);
//   - 10/12 -> treat 16-bit container values as left-justified deep-color
//     samples (common when a 10/12-bit frame is muxed into PNG16);
//   - 16 -> use the raw 16-bit range.
func LoadFrame(path string, bitDepth int) (*Frame, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	img, _, err := image.Decode(file)
	if err != nil {
		return nil, fmt.Errorf("decode %s: %w", path, err)
	}

	bounds := img.Bounds()
	w, h := bounds.Dx(), bounds.Dy()
	n := w * h
	frame := &Frame{
		Name:     path,
		Width:    w,
		Height:   h,
		BitDepth: bitDepth,
		R:        make([]uint32, n),
		G:        make([]uint32, n),
		B:        make([]uint32, n),
	}

	autoDepth := 0
	idx := 0
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			r16, g16, b16, _ := img.At(x, y).RGBA()
			if autoDepth == 0 {
				if _, ok := img.At(x, y).(color.Gray); ok {
					autoDepth = 8
				} else {
					autoDepth = inferContainerDepth(img)
				}
			}
			frame.R[idx] = r16
			frame.G[idx] = g16
			frame.B[idx] = b16
			idx++
		}
	}

	depth := bitDepth
	if depth == 0 {
		depth = autoDepth
	}
	frame.BitDepth = depth
	normalize(frame, depth)
	return frame, nil
}

func inferContainerDepth(img image.Image) int {
	switch img.(type) {
	case *image.Gray, *image.RGBA, *image.Paletted:
		return 8
	default:
		// image.RGBA64 / image.NRGBA64 / image.Alpha16 etc.
		return 16
	}
}

// normalize converts 16-bit container samples (image.Image RGBA() always
// returns 16-bit-premultiplied values) into the requested signal bit depth.
func normalize(f *Frame, depth int) {
	if depth == 16 {
		return
	}
	shift := uint(16 - depth)
	for i := range f.R {
		f.R[i] >>= shift
		f.G[i] >>= shift
		f.B[i] >>= shift
	}
}
