package sync

import "time"

// DemoSpec configures a deterministic interview trace used by tests and the CLI.
type DemoSpec struct {
	FPS         float64
	TotalFrames int64
	ChangeFrame int64
	Offset      time.Duration
	Pattern     string
}

// DemoRecords builds five-stage wide records. The skip pattern places the
// discontinuity at the edit stage and preserves it through render/playback.
func DemoRecords(spec DemoSpec) []Record {
	opts := Options{FPS: spec.FPS}.normalized()
	frameDuration := int64(opts.FrameDuration)
	total := spec.TotalFrames
	if total <= 0 {
		total = 32
	}
	change := spec.ChangeFrame
	if change <= 0 || change >= total {
		change = total / 2
	}
	offset := int64(spec.Offset)
	if offset == 0 {
		offset = 6 * frameDuration
	}
	pattern := spec.Pattern
	if pattern == "" {
		pattern = KindFrameSkip
	}

	records := make([]Record, 0, total*int64(len(Stages)))
	for frame := int64(0); frame < total; frame++ {
		base := frame * frameDuration
		video := base
		captureAudio := base
		ptsAudio := base

		editAudio := base
		switch pattern {
		case KindFixedOffset:
			editAudio = base + offset
		case KindLinearDrift:
			if frame < change {
				break
			}
			denominator := total - 1 - change
			if denominator <= 0 {
				denominator = 1
			}
			progress := frame - change
			if progress > denominator {
				progress = denominator
			}
			editAudio = base + int64(float64(offset)*float64(progress)/float64(denominator))
		default:
			if frame >= change {
				editAudio = base + offset
			}
		}

		positions := map[Stage][2]int64{
			StageCapture:  {video, captureAudio},
			StagePTS:      {video, ptsAudio},
			StageEdit:     {video, editAudio},
			StageRender:   {video, editAudio},
			StagePlayback: {video, editAudio},
		}
		for _, stage := range Stages {
			frameValue := frame
			videoValue := positions[stage][0]
			audioValue := positions[stage][1]
			records = append(records, Record{
				Stage:      stage,
				Frame:      &frameValue,
				WallNS:     frame * frameDuration,
				VideoPosNS: &videoValue,
				AudioPosNS: &audioValue,
			})
		}
	}
	return records
}
