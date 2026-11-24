package mixloud

type ProcessedAudio struct {
	ID    string
	Label string
	Audio *Audio
}

type Comparison struct {
	Report Report
	Audio  []ProcessedAudio
}

func Compare(mix *Audio, stems StemSet, inputName string, opts Options) (*Comparison, error) {
	opts = opts.normalized()
	if mix == nil {
		if stems.Dialogue == nil && stems.Music == nil && stems.SFX == nil {
			return nil, errNoAudio
		}
		mix = SumStems(stems)
	}

	dialogueStem := stems.Dialogue
	dialoguePriority := DialoguePriorityMix(mix, opts, dialogueStem)
	usingStems := stems.Dialogue != nil && (stems.Music != nil || stems.SFX != nil)
	if usingStems {
		stemMix, err := StemDialoguePriority(stems, opts)
		if err != nil {
			return nil, err
		}
		dialoguePriority = stemMix
	}

	processed := []ProcessedAudio{
		{ID: "bypass", Label: "不处理", Audio: mix.Clone()},
		{ID: "peak_limit", Label: "峰值限制", Audio: PeakLimit(mix, opts.PeakLimitDBFS, 5)},
		{ID: "dialogue_priority", Label: "对白优先", Audio: dialoguePriority},
	}

	report := Report{Input: inputName, Options: opts}
	if usingStems {
		report.Notes = append(report.Notes, "对白优先使用分轨包络对音乐/效果声做侧链闪避。")
	} else if dialogueStem != nil && mix.NumChannels() == 2 {
		report.Notes = append(report.Notes, "使用对白分轨判定活动窗，并在混音上衰减对应区间的侧场能量。")
	} else if mix.NumChannels() == 2 {
		report.Notes = append(report.Notes, "未提供可用分轨：对白优先使用中置/侧场启发式，仅衰减对白窗内的侧场能量。")
	} else {
		report.Notes = append(report.Notes, "单声道素材无法用中置/侧场分离对白；对白优先方案保持透传，建议提供对白/音乐/效果声分轨。")
	}
	report.Notes = append(report.Notes, "整合响度采用 BS.1770 K 加权与双门限；400 ms 窗默认 100 ms 步进，LRA(400ms) 为题目口径，另附 3 秒块 EBU 风格 LRA。")

	for _, item := range processed {
		schemeReport, err := AnalyzeScheme(item.Audio, item.ID, item.Label, opts, dialogueStem)
		if err != nil {
			return nil, err
		}
		report.Schemes = append(report.Schemes, *schemeReport)
	}

	stemDefs := []struct {
		id    string
		label string
		audio *Audio
		self  bool
	}{
		{"stem_dialogue", "对白分轨", stems.Dialogue, true},
		{"stem_music", "音乐分轨", stems.Music, false},
		{"stem_sfx", "效果声分轨", stems.SFX, false},
	}
	for _, def := range stemDefs {
		if def.audio == nil {
			continue
		}
		var selfStem *Audio
		if def.self {
			selfStem = def.audio
		}
		stemReport, err := AnalyzeScheme(def.audio, def.id, def.label, opts, selfStem)
		if err != nil {
			return nil, err
		}
		report.Stems = append(report.Stems, *stemReport)
	}
	report.Verdicts = conclusions(&report)
	return &Comparison{Report: report, Audio: processed}, nil
}

func SumStems(stems StemSet) *Audio {
	var base *Audio
	for _, candidate := range []*Audio{stems.Dialogue, stems.Music, stems.SFX} {
		if candidate != nil {
			base = candidate
			break
		}
	}
	if base == nil {
		return nil
	}
	out, _ := NewAudio(base.SampleRate, base.NumChannels(), base.Frames())
	for _, stem := range []*Audio{stems.Dialogue, stems.Music, stems.SFX} {
		if stem == nil || stem.SampleRate != base.SampleRate || stem.Frames() != base.Frames() || stem.NumChannels() != base.NumChannels() {
			continue
		}
		for ch := 0; ch < out.NumChannels(); ch++ {
			for i, x := range stem.Channels[ch] {
				out.Channels[ch][i] += x
			}
		}
	}
	return out
}
