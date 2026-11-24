package mixloud

import (
	"errors"
	"math"
)

var (
	errDialogueStemRequired = errors.New("dialogue stem is required")
	errStemMismatch         = errors.New("stems must have matching sample rate and length")
	errStemChannelMismatch  = errors.New("stems must have matching channel counts")
	errNoAudio              = errors.New("no audio was provided")
)

// PeakLimit applies stereo-linked lookahead peak limiting. The returned audio
// is guaranteed not to exceed the linear threshold (up to float rounding).
func PeakLimit(a *Audio, thresholdDB float64, lookaheadMS float64) *Audio {
	out := a.Clone()
	n := out.Frames()
	if n == 0 {
		return out
	}
	threshold := db2gain(thresholdDB)
	if threshold <= 0 {
		threshold = 1
	}
	lookahead := int(math.Round(float64(out.SampleRate) * lookaheadMS / 1000))
	if lookahead < 1 {
		lookahead = 1
	}

	required := make([]float64, n)
	for i := 0; i < n; i++ {
		var peak float64
		for ch := 0; ch < out.NumChannels(); ch++ {
			if x := math.Abs(out.Channels[ch][i]); x > peak {
				peak = x
			}
		}
		required[i] = 1
		if peak > threshold {
			required[i] = threshold / peak
		}
	}

	future := slidingFutureMin(required, lookahead)
	attackAlpha := 1 - math.Exp(-1/(float64(out.SampleRate)*0.0005))
	releaseAlpha := 1 - math.Exp(-1/(float64(out.SampleRate)*0.080))
	gain := 1.0
	for i := 0; i < n; i++ {
		target := future[i]
		alpha := releaseAlpha
		if target < gain {
			alpha = attackAlpha
		}
		gain += alpha * (target - gain)
		if required[i] < gain {
			gain = required[i]
		}
		for ch := 0; ch < out.NumChannels(); ch++ {
			out.Channels[ch][i] *= gain
		}
	}
	return out
}

func slidingFutureMin(xs []float64, lookahead int) []float64 {
	n := len(xs)
	out := make([]float64, n)
	deque := make([]int, 0, n)
	right := -1
	for i := 0; i < n; i++ {
		end := minInt(n-1, i+lookahead)
		for right < end {
			right++
			j := right
			for len(deque) > 0 && xs[deque[len(deque)-1]] >= xs[j] {
				deque = deque[:len(deque)-1]
			}
			deque = append(deque, j)
		}
		for len(deque) > 0 && deque[0] < i {
			deque = deque[1:]
		}
		if len(deque) > 0 {
			out[i] = xs[deque[0]]
		} else {
			out[i] = xs[i]
		}
	}
	return out
}

// DialoguePriorityMix keeps the center (normally dialogue) unchanged and
// ducks side energy during dialogue-active 400 ms regions. For mono input it
// returns a clone because a single channel cannot separate dialogue from
// music/effects; use StemDialoguePriority when stems are available.
func DialoguePriorityMix(a *Audio, opts Options, dialogueStem *Audio) *Audio {
	opts = opts.normalized()
	out := a.Clone()
	if out.NumChannels() != 2 {
		return out
	}

	filtered := applyKWeight(a)
	calcs := buildWindows(a, filtered, opts, dialogueStem)
	target := make([]float64, out.Frames())
	duck := db2gain(opts.DialogueDuckDB)
	for i := range target {
		target[i] = 1
	}
	for _, c := range calcs {
		if !c.window.Dialogue {
			continue
		}
		start := int(c.window.StartSec * float64(out.SampleRate))
		end := int(c.window.EndSec * float64(out.SampleRate))
		start = maxInt(0, start)
		end = minInt(out.Frames(), end)
		for i := start; i < end; i++ {
			target[i] = duck
		}
	}

	smooth := 1.0
	smoothAlpha := 1 - math.Exp(-1/(float64(out.SampleRate)*0.030))
	left, right := out.Channels[0], out.Channels[1]
	for i := 0; i < out.Frames(); i++ {
		smooth += smoothAlpha * (target[i] - smooth)
		mid := (left[i] + right[i]) / 2
		side := (left[i] - right[i]) / 2
		left[i] = mid + side*smooth
		right[i] = mid - side*smooth
	}
	return out
}

// StemSet contains optional separately labeled mix sources.
type StemSet struct {
	Dialogue *Audio
	Music    *Audio
	SFX      *Audio
}

// StemDialoguePriority ducks music and SFX using the dialogue stem envelope.
func StemDialoguePriority(stems StemSet, opts Options) (*Audio, error) {
	if stems.Dialogue == nil {
		return nil, errDialogueStemRequired
	}
	base := stems.Dialogue
	for _, candidate := range []*Audio{stems.Music, stems.SFX} {
		if candidate == nil {
			continue
		}
		if candidate.SampleRate != base.SampleRate || candidate.Frames() != base.Frames() {
			return nil, errStemMismatch
		}
		if candidate.NumChannels() != base.NumChannels() {
			return nil, errStemChannelMismatch
		}
	}
	opts = opts.normalized()
	out, err := NewAudio(base.SampleRate, base.NumChannels(), base.Frames())
	if err != nil {
		return nil, err
	}
	duck := db2gain(opts.DialogueDuckDB)
	frame := base.SampleRate / 100
	if frame < 1 {
		frame = 1
	}
	gain := 1.0
	alpha := 1 - math.Exp(-1/(float64(base.SampleRate)*0.080))
	for start := 0; start < base.Frames(); start += frame {
		end := minInt(base.Frames(), start+frame)
		var energy, count float64
		for ch := 0; ch < base.NumChannels(); ch++ {
			for i := start; i < end; i++ {
				x := base.Channels[ch][i]
				energy += x * x
				count++
			}
		}
		rmsDB := safeDB(math.Sqrt(energy / count))
		target := 1.0
		if finite(rmsDB) && rmsDB > -50 {
			target = duck
		}
		for i := start; i < end; i++ {
			gain += alpha * (target - gain)
			for ch := 0; ch < out.NumChannels(); ch++ {
				out.Channels[ch][i] = base.Channels[ch][i]
				if stems.Music != nil {
					out.Channels[ch][i] += stems.Music.Channels[ch][i] * gain
				}
				if stems.SFX != nil {
					out.Channels[ch][i] += stems.SFX.Channels[ch][i] * gain
				}
			}
		}
	}
	return out, nil
}
