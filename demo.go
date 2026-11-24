package mixloud

import (
	"math"
	"math/rand"
)

const demoSampleRate = 48000
const demoDurationSec = 16.0

func DemoAudio() (*Audio, StemSet) {
	frames := int(demoSampleRate * demoDurationSec)
	dialogue, _ := NewAudio(demoSampleRate, 2, frames)
	music, _ := NewAudio(demoSampleRate, 2, frames)
	sfx, _ := NewAudio(demoSampleRate, 2, frames)

	fillDemoDialogue(dialogue)
	fillDemoMusic(music)
	fillDemoSFX(sfx)

	stems := StemSet{Dialogue: dialogue, Music: music, SFX: sfx}
	return SumStems(stems), stems
}

func fillDemoDialogue(a *Audio) {
	active := []interval{{0, 4}, {5, 8.4}, {10.5, 13.4}}
	for i := 0; i < a.Frames(); i++ {
		t := float64(i) / float64(a.SampleRate)
		in, ok := intervalAt(t, active)
		if !ok {
			continue
		}
		wordGate := math.Pow(0.5+0.5*math.Sin(2*math.Pi*4.1*t+0.7), 1.7)
		edge := math.Min(1, (t-in.start)/0.08)
		edge = math.Min(edge, math.Max(0, (in.end-t)/0.12))
		env := (0.25 + 0.75*wordGate) * edge
		f0 := 132 + 18*math.Sin(2*math.Pi*0.7*t)
		voice := 0.58*math.Sin(2*math.Pi*f0*t) +
			0.24*math.Sin(2*math.Pi*2*f0*t) +
			0.10*math.Sin(2*math.Pi*3.0*f0*t) +
			0.05*math.Sin(2*math.Pi*5.2*f0*t)
		x := 0.34 * env * voice
		a.Channels[0][i] = x
		a.Channels[1][i] = x
	}
}

func fillDemoMusic(a *Audio) {
	chord := []float64{220.0, 277.18, 329.63, 440.0}
	for i := 0; i < a.Frames(); i++ {
		t := float64(i) / float64(a.SampleRate)
		if t >= 4 && t < 5 {
			continue
		}
		if t >= 13.5 && t < 15.5 {
			x := 0.20 * (math.Sin(2*math.Pi*392*t) + math.Sin(2*math.Pi*493.88*t) + math.Sin(2*math.Pi*587.33*t)) / 3
			env := math.Min(1, (t-13.5)/0.05) * math.Min(1, (15.5-t)/0.08)
			a.Channels[0][i] = x * env
			a.Channels[1][i] = -x * env
			continue
		}
		if t >= 8.5 && t < 10.5 {
			ad := 0.0
			for k, f := range chord {
				ad += math.Sin(2 * math.Pi * f * (1 + 0.002*math.Sin(7*t)) * t)
				ad += 0.35 * math.Sin(2*math.Pi*2*f*t)
				_ = k
			}
			ad /= 5.4
			ad = math.Tanh(2.8*ad) / math.Tanh(2.8)
			edge := math.Min(1, (t-8.5)/0.02) * math.Min(1, (10.5-t)/0.05)
			a.Channels[0][i] = 0.97 * ad * edge
			a.Channels[1][i] = 0.94 * (ad + 0.03*math.Sin(2*math.Pi*1760*t)) * edge
			continue
		}

		bed := 0.0
		for _, f := range chord[:3] {
			bed += math.Sin(2*math.Pi*f*t) + 0.25*math.Sin(2*math.Pi*2.01*f*t)
		}
		bed /= 3.75
		tremolo := 0.82 + 0.18*math.Sin(2*math.Pi*0.35*t)
		edge := 1.0
		if t < 0.4 {
			edge = t / 0.4
		}
		if t > 15.6 {
			edge = math.Max(0, (demoDurationSec-t)/0.4)
		}
		a.Channels[0][i] = 0.105 * bed * tremolo * edge
		a.Channels[1][i] = 0.095 * math.Sin(2*math.Pi*277.8*t) * tremolo * edge
	}
}

func fillDemoSFX(a *Audio) {
	rng := rand.New(rand.NewSource(42))
	start := 6.2
	length := 0.85
	var low float64
	for i := 0; i < a.Frames(); i++ {
		t := float64(i) / float64(a.SampleRate)
		if t < start || t >= start+length {
			continue
		}
		u := (t - start) / length
		noise := rng.Float64()*2 - 1
		low += 0.12 * (noise - low)
		boom := math.Sin(2 * math.Pi * (72 - 28*u) * (t - start))
		env := math.Exp(-4.2 * u)
		x := 1.55 * env * (0.72*low + 0.55*boom)
		if x > 0.985 {
			x = 0.985
		} else if x < -0.985 {
			x = -0.985
		}
		a.Channels[0][i] = x
		a.Channels[1][i] = 0.92 * x
	}
}

type interval struct{ start, end float64 }

func insideInterval(t float64, intervals []interval) bool {
	_, ok := intervalAt(t, intervals)
	return ok
}

func intervalAt(t float64, intervals []interval) (interval, bool) {
	for _, in := range intervals {
		if t >= in.start && t < in.end {
			return in, true
		}
	}
	return interval{}, false
}

func floorIntervalStart(t float64, intervals []interval) float64 {
	for _, in := range intervals {
		if t >= in.start && t < in.end {
			return in.start
		}
	}
	return 0
}
