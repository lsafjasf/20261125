package freezedetect

import (
	"image"
	"image/color"
)

// Image 复用标准库图像接口，整个检测链路不依赖 cgo 或第三方解码库。
type Image = image.Image

type frameMetrics struct {
	gray      []uint8
	lumaMean  float64
	darkRatio float64
	w, h      int
}

func grayAt(img Image, x, y int) uint8 {
	c := img.At(x, y)
	g := color.GrayModel.Convert(c).(color.Gray)
	return g.Y
}

func sampleMetrics(img Image, darkLuma uint8) frameMetrics {
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	gray := make([]uint8, w*h)
	var sum, dark int
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			v := grayAt(img, b.Min.X+x, b.Min.Y+y)
			gray[y*w+x] = v
			sum += int(v)
			if v < darkLuma {
				dark++
			}
		}
	}
	n := float64(w * h)
	return frameMetrics{
		gray:      gray,
		lumaMean:  float64(sum) / n,
		darkRatio: float64(dark) / n,
		w:         w,
		h:         h,
	}
}

// compareMetrics 计算两帧之间的差异证据。
type diffMetrics struct {
	meanDiff      float64
	changedRatio  float64
	noiseVariance float64
	lumaDelta     float64
}

func compareFrames(prev, cur frameMetrics, cfg Config) diffMetrics {
	n := len(cur.gray)
	if n == 0 || len(prev.gray) != n {
		return diffMetrics{}
	}
	var diffSum, changed, darkCount int
	var noiseSq float64
	changeStep := uint8(6)
	for i, v := range cur.gray {
		p := prev.gray[i]
		d := int(v) - int(p)
		if d < 0 {
			d = -d
		}
		diffSum += d
		if d >= int(changeStep) {
			changed++
		}
		if v < cfg.DarkLuma {
			darkCount++
			res := float64(int(v) - int(p))
			noiseSq += res * res
		}
	}
	fn := float64(n)
	dm := diffMetrics{
		meanDiff:     float64(diffSum) / fn,
		changedRatio: float64(changed) / fn,
		lumaDelta:    cur.lumaMean - prev.lumaMean,
	}
	if darkCount > 0 {
		dm.noiseVariance = noiseSq / float64(darkCount)
	}
	return dm
}

// imagesEqual 比较两张图（用于字幕层），尺寸不同视为发生变化。
func imagesEqual(a, b Image) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	ab, bb := a.Bounds(), b.Bounds()
	if ab.Dx() != bb.Dx() || ab.Dy() != bb.Dy() {
		return false
	}
	for y := 0; y < ab.Dy(); y++ {
		for x := 0; x < ab.Dx(); x++ {
			if grayAt(a, ab.Min.X+x, ab.Min.Y+y) != grayAt(b, bb.Min.X+x, bb.Min.Y+y) {
				return false
			}
		}
	}
	return true
}
