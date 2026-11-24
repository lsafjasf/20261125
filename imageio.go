package nightwb

import (
	"image"
	"image/color"
	"image/png"
	"io"
)

// DecodeFrame converts any decoded image into linear-light sRGB, recording
// per-channel source clipping (at the type's maximum) and crushing (at zero).
func DecodeFrame(img image.Image) *Frame {
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	f := &Frame{
		Width:          w,
		Height:         h,
		RGB:            make([][3]float64, w*h),
		ChannelClipped: make([][3]bool, w*h),
		ChannelCrushed: make([][3]bool, w*h),
	}

	p := 0
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			r, g, bl, _ := img.At(x, y).RGBA()
			vals := [3]uint32{r, g, bl}
			// RGBA() always returns 16-bit scaled values; the actual encoding
			// maximum depends on the concrete pixel type (8-bit models max at
			// 0xff<<8, 16-bit models at 0xffff).
			maxVal := float64(0xffff)
			switch img.At(x, y).(type) {
			case color.RGBA, color.NRGBA, color.Gray, color.Alpha:
				maxVal = 0xff << 8
			}
			for c, v := range vals {
				lin := SRGBToLinear(float64(v) / maxVal)
				f.RGB[p][c] = lin
				f.ChannelClipped[p][c] = float64(v) >= maxVal
				f.ChannelCrushed[p][c] = v == 0
			}
			p++
		}
	}
	return f
}

// EncodePNG writes an adapted linear frame as 8-bit sRGB PNG.
func EncodePNG(w io.Writer, adapted *AdaptedFrame, width, height int) error {
	img := image.NewNRGBA(image.Rect(0, 0, width, height))
	for i, px := range adapted.RGB {
		o := i * 4
		img.Pix[o+0] = clamp8(LinearToSRGB(px[0]))
		img.Pix[o+1] = clamp8(LinearToSRGB(px[1]))
		img.Pix[o+2] = clamp8(LinearToSRGB(px[2]))
		img.Pix[o+3] = 255
	}
	return png.Encode(w, img)
}

// EncodeMaskPNG writes a 16-bit mask as a grayscale PNG (flag bits preserved;
// viewers show it as a grayscale heat map of defect severity).
func EncodeMaskPNG(w io.Writer, mask []uint16, width, height int) error {
	img := image.NewGray16(image.Rect(0, 0, width, height))
	copy(img.Pix, maskBytes(mask))
	return png.Encode(w, img)
}

func maskBytes(mask []uint16) []byte {
	out := make([]byte, len(mask)*2)
	for i, v := range mask {
		out[i*2] = byte(v >> 8)
		out[i*2+1] = byte(v)
	}
	return out
}

func clamp8(v float64) uint8 {
	v = clamp01(v)
	return uint8(v*255 + 0.5)
}
