package freezedetect

import (
	"image"
	"image/color"
	"math/rand"
)

const testW, testH = 48, 32

// makeStillFrame 生成一帧正常静物画面：背景渐变 + 固定亮块，逐帧完全一致。
func makeStillFrame() *image.Gray {
	img := image.NewGray(image.Rect(0, 0, testW, testH))
	for y := 0; y < testH; y++ {
		for x := 0; x < testW; x++ {
			v := uint8((x + y) % 200)
			img.SetGray(x, y, color.Gray{Y: v})
		}
	}
	for y := 6; y < 14; y++ {
		for x := 6; x < 18; x++ {
			img.SetGray(x, y, color.Gray{Y: 220})
		}
	}
	return img
}

// makeDarkFrame 生成低照静物画面（整体灰度低于 DarkLuma 阈值）。
func makeDarkFrame() *image.Gray {
	img := image.NewGray(image.Rect(0, 0, testW, testH))
	for y := 0; y < testH; y++ {
		for x := 0; x < testW; x++ {
			img.SetGray(x, y, color.Gray{Y: uint8(4 + (x+y)%8)})
		}
	}
	return img
}

// addNoise 返回深拷贝并对每个像素叠加 [-amp, amp] 范围的传感器噪声。
func addNoise(src *image.Gray, rng *rand.Rand, amp int) *image.Gray {
	out := src
	px := make([]uint8, len(src.Pix))
	copy(px, src.Pix)
	out = &image.Gray{Pix: px, Stride: src.Stride, Rect: src.Rect}
	for i := range out.Pix {
		n := rng.Intn(2*amp+1) - amp
		v := int(out.Pix[i]) + n
		switch {
		case v < 0:
			v = 0
		case v > 255:
			v = 255
		}
		out.Pix[i] = uint8(v)
	}
	return out
}

// shiftLuma 返回深拷贝并整体平移亮度，用于制造闪烁帧。
func shiftLuma(src *image.Gray, delta int) *image.Gray {
	px := make([]uint8, len(src.Pix))
	copy(px, src.Pix)
	for i, v := range px {
		c := int(v) + delta
		switch {
		case c < 0:
			c = 0
		case c > 255:
			c = 255
		}
		px[i] = uint8(c)
	}
	return &image.Gray{Pix: px, Stride: src.Stride, Rect: src.Rect}
}

// movingFrame 生成与静物帧有实质差异的运动画面。
func movingFrame(step int) *image.Gray {
	img := makeStillFrame()
	for y := 0; y < testH; y++ {
		for x := 0; x < testW; x++ {
			img.SetGray(x, y, color.Gray{Y: uint8((x*3 + y + step*7) % 256)})
		}
	}
	return img
}

// subtitleBar 生成字幕条，text 变化时像素内容随之变化。
func subtitleBar(textID int) *image.Gray {
	img := image.NewGray(image.Rect(0, 0, testW, 8))
	for x := 2; x < testW-2; x++ {
		if (x+textID*5)%11 < 5 {
			for y := 2; y < 6; y++ {
				img.SetGray(x, y, color.Gray{Y: 235})
			}
		}
	}
	return img
}
