package lutdiff

// Chart geometry. The top region is a hue x saturation plane; the bottom
// holds six full-width ramps (neutral, R, G, B, near-black, near-white).
const (
	ChartW       = 900
	ChartH       = 600
	ChartPlaneH  = 360
	ChartRampTop = 372
	ChartRampH   = 30
	ChartRampGap = 8
	ChartRampN   = 6
)

// RampNames matches the top-to-bottom ramp order in SourceChart.
var RampNames = []string{
	"中性灰阶",
	"纯红渐变",
	"纯绿渐变",
	"纯蓝渐变",
	"近黑边界（0→0.06）",
	"近白边界（0.82→1）",
}

func rampColor(row int, t float64) RGB {
	switch row {
	case 0:
		return RGB{t, t, t}
	case 1:
		return RGB{t, 0, 0}
	case 2:
		return RGB{0, t, 0}
	case 3:
		return RGB{0, 0, t}
	case 4:
		k := 0.06 * t
		return RGB{k, 0.75 * k, 0.9 * k}
	default:
		base := RGB{0.82, 0.78, 0.74}
		return RGB{
			base[0] + t*(1-base[0]),
			base[1] + t*(1-base[1]),
			base[2] + t*(1-base[2]),
		}
	}
}

// SourceChart builds the procedural test chart used for the image artifacts.
func SourceChart() *FloatImage {
	img := NewFloatImage(ChartW, ChartH)
	for y := 0; y < ChartH; y++ {
		for x := 0; x < ChartW; x++ {
			var c RGB
			switch {
			case y < ChartPlaneH:
				hue := float64(x) / float64(ChartW-1)
				sat := 1 - float64(y)/float64(ChartPlaneH-1)
				c = HSLToRGB(hue, sat, 0.5)
			default:
				row := (y - ChartRampTop) / (ChartRampH + ChartRampGap)
				if row < 0 {
					row = 0
				}
				if row >= ChartRampN {
					row = ChartRampN - 1
				}
				rY0 := ChartRampTop + row*(ChartRampH+ChartRampGap)
				if y >= rY0 && y < rY0+ChartRampH {
					t := float64(x) / float64(ChartW-1)
					c = rampColor(row, t)
				} else {
					c = RGB{0.12, 0.12, 0.12}
				}
			}
			img.set(x, y, c)
		}
	}
	return img
}
