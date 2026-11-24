package analyze

import (
	"math"

	"surrounddiag/internal/wav"
)

func (run *analyzer) referenceDistributionFindings(reference *wav.File, mappings []ChannelMapping) []Finding {
	if len(run.windows) == 0 {
		return nil
	}
	refWindows := make([][]WindowChannel, len(run.windows))
	for wi, window := range run.windows {
		refWindows[wi] = make([]WindowChannel, reference.Channels)
		start := int(math.Round(window.Start * float64(reference.SampleRate)))
		end := int(math.Round(window.End * float64(reference.SampleRate)))
		end = minInt(end, reference.Frames)
		for channel := 0; channel < reference.Channels; channel++ {
			if end <= start {
				continue
			}
			samples := reference.Channel(channel)
			stats := statsFor(samples[start:end])
			fillBands(&stats, samples[start:end], reference.SampleRate)
			refWindows[wi][channel] = WindowChannel{
				Index: channel, Name: reference.Layout[channel], RMSDB: stats.RMSDB,
				PeakDB: stats.PeakDB, LowRatio: stats.LowRatio,
				VoiceRatio: stats.VoiceRatio, HighRatio: stats.HighRatio,
			}
		}
	}
	var findings []Finding
	for channel := 0; channel < run.file.Channels; channel++ {
		mapping := mappings[channel]
		refChannel := channel
		if mapping.ReferenceIndex >= 0 {
			refChannel = mapping.ReferenceIndex
		}
		var lowLoss, lowGain, surroundLoss, centerLoss []bool
		lowLoss = make([]bool, len(run.windows))
		lowGain = make([]bool, len(run.windows))
		surroundLoss = make([]bool, len(run.windows))
		centerLoss = make([]bool, len(run.windows))
		name := run.file.Layout[channel]
		for wi, window := range run.windows {
			out := window.Channels[channel]
			ref := refWindows[wi][refChannel]
			if out.RMSDB < -85 || ref.RMSDB < -60 {
				continue
			}
			lowRatioDB := dbRatio(lowEnergy(out), lowEnergy(ref))
			switch {
			case name == "LFE" && lowRatioDB < -7:
				lowLoss[wi] = true
			case name == "FC" && lowRatioDB < -10:
				lowLoss[wi] = true
			case (name == "FL" || name == "FR") && lowRatioDB < -9:
				lowLoss[wi] = true
			}
			if name != "LFE" && lowRatioDB > 10 {
				lowGain[wi] = true
			}
			if (name == "BL" || name == "BR" || name == "SL" || name == "SR") &&
				out.RMSDB-ref.RMSDB < -12 {
				surroundLoss[wi] = true
			}
			if name == "FC" {
				voiceRatioDB := (out.RMSDB - ref.RMSDB) + 10*math.Log10(math.Max(out.VoiceRatio, 1e-8)/math.Max(ref.VoiceRatio, 1e-8))
				if voiceRatioDB < -10 {
					centerLoss[wi] = true
				}
			}
		}
		findings = append(findings, findingFromFlags(run, lowLoss, Finding{
			Severity: "warning", Code: "reference_low_frequency_loss",
			Channels:   []string{name},
			RootCause:  "低频总线被高通、衰减、错误下混或未送入该声道。",
			Confidence: 0.84,
			Evidence:   "20–120 Hz 频带能量相对参考母版显著下降。",
		}, "低频能量相对参考母版明显减少。")...)
		findings = append(findings, findingFromFlags(run, lowGain, Finding{
			Severity: "warning", Code: "reference_low_frequency_gain",
			Channels:   []string{name},
			RootCause:  "低频被错误串入该声道，常见于 LFE/FC 互换或 bass management 路由错误。",
			Confidence: 0.76,
			Evidence:   "20–120 Hz 频带能量相对参考母版显著增加。",
		}, "低频能量相对参考母版明显增加。")...)
		findings = append(findings, findingFromFlags(run, surroundLoss, Finding{
			Severity: "high", Code: "reference_surround_loss",
			Channels:   []string{name},
			RootCause:  "环绕分轨被衰减或漏路由，导致包围感消失。",
			Confidence: 0.88,
			Evidence:   "该环绕声道窗口 RMS 比参考母版低超过 12 dB。",
		}, "环绕电平相对参考母版明显降低。")...)
		findings = append(findings, findingFromFlags(run, centerLoss, Finding{
			Severity: "high", Code: "reference_center_dialogue_loss",
			Channels:   []string{"FC"},
			RootCause:  "中心对白被折叠、衰减或相位抵消，导致对白发虚。",
			Confidence: 0.86,
			Evidence:   "中心声道语音频带能量相对参考母版下降超过 10 dB。",
		}, "中心声道语音频带能量相对参考母版明显降低。")...)
	}
	return findings
}

func lowEnergy(channel WindowChannel) float64 {
	return math.Pow(10, channel.RMSDB/10) * channel.LowRatio
}

func dbRatio(value, reference float64) float64 {
	if value <= 1e-12 || reference <= 1e-12 {
		return -120
	}
	return 10 * math.Log10(value/reference)
}
