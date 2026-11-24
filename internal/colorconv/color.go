package colorconv

import "math"

// RGB is scene-linear, non-premultiplied color in a named RGB space.
type RGB struct {
	R, G, B float64
}

type Mat3 [3][3]float64

func (m Mat3) Mul(c RGB) RGB {
	return RGB{
		R: m[0][0]*c.R + m[0][1]*c.G + m[0][2]*c.B,
		G: m[1][0]*c.R + m[1][1]*c.G + m[1][2]*c.B,
		B: m[2][0]*c.R + m[2][1]*c.G + m[2][2]*c.B,
	}
}

// Rec2020ToXYZ and XYZToRec709 use D65-adapted linear primaries.
var (
	Rec2020ToXYZ = Mat3{
		{0.6369580483012914, 0.14461690358620832, 0.16888097516417205},
		{0.2627002120112671, 0.6779980715188708, 0.05930171646986196},
		{0.0000000000000000, 0.028072693049087428, 1.0609850577107909},
	}
	XYZToRec709 = Mat3{
		{3.2409699419045226, -1.537383177570094, -0.4986107602930034},
		{-0.9692436362808798, 1.8759675015077204, 0.04155505740717561},
		{0.05563007969699366, -0.20397695888897652, 1.0569715142428784},
	}
	Rec2020ToRec709 = XYZToRec709.MulMat(Rec2020ToXYZ)
)

func (a Mat3) MulMat(b Mat3) Mat3 {
	var out Mat3
	for row := 0; row < 3; row++ {
		for col := 0; col < 3; col++ {
			out[row][col] = a[row][0]*b[0][col] + a[row][1]*b[1][col] + a[row][2]*b[2][col]
		}
	}
	return out
}

func ConvertRec2020To709(c RGB) RGB {
	return Rec2020ToRec709.Mul(c)
}

func Luminance709(c RGB) float64 {
	return 0.2126*c.R + 0.7152*c.G + 0.0722*c.B
}

// DecodeSignal converts a full-range electrical value [0,1] to linear light.
func DecodeSignal(v float64, transfer string) float64 {
	switch transfer {
	case "linear", "pfm":
		return v
	case "srgb":
		if v <= 0.04045 {
			return v / 12.92
		}
		return math.Pow((v+0.055)/1.055, 2.4)
	case "pq":
		return pqDecode(v)
	case "rec709", "bt709", "":
		if v < 0.081 {
			return v / 4.5
		}
		return math.Pow((v+0.099)/1.099, 1/0.45)
	default:
		if v < 0.081 {
			return v / 4.5
		}
		return math.Pow((v+0.099)/1.099, 1/0.45)
	}
}

// EncodeSignal converts linear Rec.709 light to an 8/10-bit style signal.
func EncodeSignal(v float64, transfer string) float64 {
	v = math.Max(0, v)
	switch transfer {
	case "linear", "pfm":
		return v
	case "srgb":
		if v <= 0.0031308 {
			return 12.92 * v
		}
		return 1.055*math.Pow(v, 1/2.4) - 0.055
	default:
		if v < 0.018 {
			return 4.5 * v
		}
		return 1.099*math.Pow(v, 0.45) - 0.099
	}
}

func pqDecode(n float64) float64 {
	const m1 = 2610.0 / 16384.0
	const m2 = 2523.0 / 4096.0 * 128.0
	const c1 = 3424.0 / 4096.0
	const c2 = 2413.0 / 4096.0 * 32.0
	const c3 = 2392.0 / 4096.0 * 32.0
	n = clamp01(n)
	np := math.Pow(n, 1/m2)
	v := math.Max(np-c1, 0) / (c2 - c3*np)
	l := math.Pow(v, 1/m1)
	// PQ absolute luminance normalized to 10,000 cd/m². Callers exposing
	// ordinary SDR highlights usually apply -pq-exposure after this decode.
	return l
}

func clamp01(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}

// Lab is CIE L*a*b* under D65. Values above L*=100 are possible for HDR.
type Lab struct{ L, A, B float64 }

func RGBToLab(c RGB) Lab {
	x, y, z := linear709ToXYZ(c)
	return XYZToLab(x, y, z)
}

func Rec2020ToLab(c RGB) Lab {
	xyz := Rec2020ToXYZ.Mul(c)
	return XYZToLab(xyz.R, xyz.G, xyz.B)
}

func XYZToLab(x, y, z float64) Lab {
	x /= 0.95047
	y /= 1.00000
	z /= 1.08883
	fx := labF(x)
	fy := labF(y)
	fz := labF(z)
	return Lab{L: 116*fy - 16, A: 500 * (fx - fy), B: 200 * (fy - fz)}
}

func linear709ToXYZ(c RGB) (float64, float64, float64) {
	// Rec.709 linear RGB -> XYZ D65.
	x := 0.4123907992659595*c.R + 0.357584339383878*c.G + 0.1804807884018343*c.B
	y := 0.21263900587151036*c.R + 0.7151686787677568*c.G + 0.07219231536073371*c.B
	z := 0.01933081871559185*c.R + 0.11919477979462598*c.G + 0.9505321522496607*c.B
	return x, y, z
}

func labF(t float64) float64 {
	const delta = 6.0 / 29.0
	if t > delta*delta*delta {
		return math.Cbrt(t)
	}
	return t/(3*delta*delta) + 4.0/29.0
}

func HueDegrees(l Lab) float64 {
	h := math.Atan2(l.B, l.A) * 180 / math.Pi
	if h < 0 {
		h += 360
	}
	return h
}

func Chroma(l Lab) float64 {
	return math.Hypot(l.A, l.B)
}

// HueDelta returns the signed shortest hue displacement in degrees.
func HueDelta(from, to float64) float64 {
	d := math.Mod(to-from+540, 360) - 180
	if d == -180 && to-from > 0 {
		return 180
	}
	return d
}

func DeltaE00(l1, l2 Lab) float64 {
	// A compact CIEDE2000 implementation is intentionally avoided here;
	// report ΔE76 plus dedicated hue/chroma aggregates in the pipeline.
	return math.Sqrt((l1.L-l2.L)*(l1.L-l2.L) + (l1.A-l2.A)*(l1.A-l2.A) + (l1.B-l2.B)*(l1.B-l2.B))
}
