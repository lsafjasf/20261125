package synth

import (
	"math"
	"math/rand"

	"rec2020-to-rec709-shoulder/internal/colorconv"
	"rec2020-to-rec709-shoulder/internal/pipeline"
)

// GenerateFrame creates a deterministic 9:16-ish Rec.2020 linear diagnostic frame.
// Frames 0..6 are a small-specular scene (frame 4 contains one-frame noise),
// while frames 7+ are a persistent bright high-texture scene.
func GenerateFrame(width, height, frame, totalFrames int) *pipeline.FloatImage {
	im := pipeline.NewFloatImage(width, height)
	brightScene := frame >= 7
	noiseFrame := frame == 4
	phase := float64(frame) / math.Max(1, float64(totalFrames-1))

	for y := 0; y < height; y++ {
		v := float64(y) / float64(height-1)
		for x := 0; x < width; x++ {
			u := float64(x) / float64(width-1)
			c := pattern(u, v, phase, brightScene, width, height)
			if noiseFrame {
				c = addNoise(c, x, y)
			}
			im.SetPixel(x, y, c)
		}
	}
	return im
}

func pattern(u, v, phase float64, bright bool, width, height int) colorconv.RGB {
	switch {
	case v < 0.24:
		return topSky(u, v, phase, bright)
	case v < 0.53:
		return gamutBars(u, v, bright)
	case v < 0.79:
		return metalTexture(u, v, phase, bright, width, height)
	default:
		return bottomRamp(u, v, phase, bright, width)
	}
}

func topSky(u, v, phase float64, bright bool) colorconv.RGB {
	t := v / 0.24
	c := colorconv.RGB{R: 0.03 + 0.10*t, G: 0.08 + 0.05*t, B: 0.30 + 0.22*t}
	orbX := 0.5 + 0.22*math.Sin(phase*2*math.Pi)
	orbY := 0.105 + 0.025*math.Cos(phase*2*math.Pi)
	d := math.Hypot((u-orbX)*1.7, (v-orbY)*4.0)
	radius := 0.10
	intensity := 2.8
	if !bright {
		radius = 0.045
		intensity = 2.2
	} else {
		intensity = 3.4
	}
	orb := math.Exp(-(d*d)/(radius*radius)) * intensity
	halo := math.Exp(-(d*d)/(radius*radius*9)) * 0.55
	c.R += orb*1.0 + halo*0.25
	c.G += orb*0.82 + halo*0.35
	c.B += orb*0.55 + halo*0.8
	return c
}

func gamutBars(u, v float64, bright bool) colorconv.RGB {
	colors := []colorconv.RGB{
		{R: 1.0}, {G: 1.0}, {B: 1.0},
		{R: 1, G: 1}, {G: 1, B: 1}, {R: 1, B: 1},
	}
	scaledU := u * 6
	idx := int(math.Floor(scaledU))
	if idx >= 6 {
		idx = 5
	}
	fu := scaledU - math.Floor(scaledU)
	fv := (v - 0.24) / (0.53 - 0.24)
	intensity := 0.25 + 1.25*smooth(fu)
	if bright {
		intensity *= 1.30 + 0.25*fv
	} else {
		intensity *= 0.30 + 0.05*fv
	}
	edgeDark := 1.0
	if fu < 0.025 || fu > 0.975 {
		edgeDark = 0.35
	}
	grating := 0.94 + 0.06*math.Sin(u*720*math.Pi)
	c := colors[idx]
	return colorconv.RGB{R: c.R * intensity * edgeDark * grating, G: c.G * intensity * edgeDark * grating, B: c.B * intensity * edgeDark * grating}
}

func metalTexture(u, v, phase float64, bright bool, width, height int) colorconv.RGB {
	t := (v - 0.53) / 0.26
	base := 0.26 + 0.20*math.Sin((u*1.7+t)*math.Pi)
	warm := 0.08 * math.Sin(u*18+phase*4)
	c := colorconv.RGB{R: base + 0.10 + warm, G: base + 0.04, B: base + 0.16 - warm}
	diagonal := 0.5 + 0.5*math.Sin((u*float64(width)*0.55+t*float64(height)*0.9)*math.Pi/3.2)
	fine := 0.5 + 0.5*math.Sin(u*float64(width)*2.1*math.Pi)
	mod := 0.72 + 0.34*diagonal + 0.10*fine
	c.R *= mod
	c.G *= mod
	c.B *= mod
	shineX := 0.25 + 0.5*phase
	shine := math.Exp(-math.Pow((u-shineX)*5.5, 2)) * math.Sin(t*math.Pi)
	gain := 1.25
	if bright {
		gain = 2.15
	} else {
		gain = 0.20
	}
	c.R += shine * gain * 1.00
	c.G += shine * gain * 0.96
	c.B += shine * gain * 0.90
	return c
}

func bottomRamp(u, v, phase float64, bright bool, width int) colorconv.RGB {
	t := (v - 0.79) / 0.21
	hue := u * 6
	idx := int(hue)
	f := hue - math.Floor(hue)
	var c colorconv.RGB
	switch idx % 6 {
	case 0:
		c = colorconv.RGB{R: 1, G: f}
	case 1:
		c = colorconv.RGB{R: 1 - f, G: 1}
	case 2:
		c = colorconv.RGB{G: 1, B: f}
	case 3:
		c = colorconv.RGB{G: 1 - f, B: 1}
	case 4:
		c = colorconv.RGB{R: f, B: 1}
	default:
		c = colorconv.RGB{R: 1, B: 1 - f}
	}
	intensity := 0.50 + 0.55*t
	if bright {
		intensity *= 1.45
	} else {
		intensity *= 0.58
	}
	gridX := math.Abs(math.Sin(u * float64(width) * 0.32 * math.Pi))
	gridY := math.Abs(math.Sin(t * 24 * math.Pi))
	grid := math.Min(gridX, gridY)
	if grid < 0.12 {
		intensity *= 0.45
	}
	moving := 0.10 * math.Sin((u*12-phase*4+t*6)*math.Pi)
	return colorconv.RGB{R: c.R*intensity + moving, G: c.G*intensity + moving, B: c.B*intensity + moving}
}

func addNoise(c colorconv.RGB, x, y int) colorconv.RGB {
	r := rand.New(rand.NewSource(int64(x*92837111 + y*689287499)))
	n := (r.Float64() - 0.5) * 0.28
	c.R = math.Max(0, c.R+n)
	c.G = math.Max(0, c.G+n)
	c.B = math.Max(0, c.B+n)
	if r.Float64() < 0.012 {
		spike := 1.8 + r.Float64()*2.5
		c.R += spike
		c.G += spike * (0.7 + r.Float64()*0.3)
		c.B += spike * (0.5 + r.Float64()*0.5)
	}
	return c
}

func smooth(t float64) float64 {
	t = math.Max(0, math.Min(1, t))
	return t * t * (3 - 2*t)
}
