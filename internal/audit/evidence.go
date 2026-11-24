package audit

import "fmt"

func railThreshold(samples uint64) uint64 {
	t := samples / 10000
	if t < 8 {
		t = 8
	}
	return t
}

func scoreLimited(s SampleStats) (int, []string) {
	if s.Samples == 0 {
		return 0, []string{"没有可用样本"}
	}
	outside := s.Counts.LT16 + s.Counts.GT235
	rail := railThreshold(s.Samples)
	at16 := s.Counts.EQ16 >= rail
	at235 := s.Counts.EQ235 >= rail
	score := 0
	var evidence []string

	switch {
	case outside == 0:
		score += 3
		evidence = append(evidence, fmt.Sprintf("支持 limited：0 个样本落在 16–235 之外（共 %d 个采样）", s.Samples))
	case s.Fractions.Outside < 0.001:
		score++
		evidence = append(evidence, fmt.Sprintf("弱支持 limited：仅 %.4f%% 样本越界，可能是量化/噪声", s.Fractions.Outside*100))
	default:
		evidence = append(evidence, fmt.Sprintf("不支持 limited：%.4f%% 样本落在 16–235 之外", s.Fractions.Outside*100))
	}
	if at16 && at235 {
		score += 3
		evidence = append(evidence, fmt.Sprintf("两端硬轨道：code 16 占 %.4f%%，code 235 占 %.4f%%", s.Fractions.EQ16*100, s.Fractions.EQ235*100))
	} else {
		if at16 {
			score++
			evidence = append(evidence, fmt.Sprintf("低端硬轨道：code 16 占 %.4f%%", s.Fractions.EQ16*100))
		}
		if at235 {
			score++
			evidence = append(evidence, fmt.Sprintf("高端硬轨道：code 235 占 %.4f%%", s.Fractions.EQ235*100))
		}
		if !at16 && !at235 {
			evidence = append(evidence, "未发现 16/235 两端的显著硬轨道")
		}
	}
	if s.Percent.P001 >= 16 && s.Percent.P999 <= 235 {
		score++
		evidence = append(evidence, fmt.Sprintf("稳健支撑 p0.1–p99.9 = %d–%d，完全位于 16–235", s.Percent.P001, s.Percent.P999))
	}
	if s.Percent.P001 >= 16 && s.Percent.P999 <= 235 && s.Percent.P999-s.Percent.P001 >= 160 {
		score++
		evidence = append(evidence, fmt.Sprintf("limited 内稳健动态跨度 %d code，接近 219 code 的全跨度", s.Percent.P999-s.Percent.P001))
	}
	return score, evidence
}

func scoreFull(s SampleStats) (int, []string) {
	if s.Samples == 0 {
		return 0, []string{"没有可用样本"}
	}
	outside := s.Counts.LT16 + s.Counts.GT235
	rail := railThreshold(s.Samples)
	score := 0
	var evidence []string

	switch {
	case s.Fractions.Outside >= 0.001:
		score += 6
		evidence = append(evidence, fmt.Sprintf("支持 full：%.4f%% 样本出现在 16–235 之外", s.Fractions.Outside*100))
	case outside > 0:
		score += 2
		evidence = append(evidence, fmt.Sprintf("弱支持 full：存在 %d 个越界样本（%.5f%%）", outside, s.Fractions.Outside*100))
	default:
		evidence = append(evidence, "不支持 full：没有 16–235 之外的样本")
	}
	if s.Counts.EQ0 >= rail {
		score += 2
		evidence = append(evidence, fmt.Sprintf("code 0 占 %.4f%%，存在 full-range 黑端锚点", s.Fractions.EQ0*100))
	}
	if s.Counts.EQ255 >= rail {
		score += 2
		evidence = append(evidence, fmt.Sprintf("code 255 占 %.4f%%，存在 full-range 白端锚点", s.Fractions.EQ255*100))
	}
	if s.Min < 16 && s.Max > 235 {
		score += 2
		evidence = append(evidence, fmt.Sprintf("同时触及 16 以下与 235 以上（min/max = %d/%d）", s.Min, s.Max))
	}
	if s.Percent.P001 < 16 || s.Percent.P999 > 235 {
		score += 2
		evidence = append(evidence, fmt.Sprintf("稳健分位越界：p0.1/p99.9 = %d/%d", s.Percent.P001, s.Percent.P999))
	}
	if s.Percent.P999-s.Percent.P001 >= 230 {
		score++
		evidence = append(evidence, fmt.Sprintf("稳健动态跨度 %d code，超过 limited 的 219 code", s.Percent.P999-s.Percent.P001))
	}
	return score, evidence
}

func rangeEvidence(s SampleStats) []string {
	if s.Samples == 0 {
		return []string{"没有可用样本，无法做出范围判断"}
	}
	return []string{
		fmt.Sprintf("样本数 %d，原始 min/max = %d/%d，唯一 code 数 = %d", s.Samples, s.Min, s.Max, s.Distinct),
		fmt.Sprintf("稳健支撑 p0.1–p99.9 = %d–%d，p1–p99 = %d–%d", s.Percent.P001, s.Percent.P999, s.Percent.P01, s.Percent.P99),
		fmt.Sprintf("越界比例：<16 = %.5f%%，>235 = %.5f%%；硬轨道：16 = %.5f%%，235 = %.5f%%", s.Fractions.LT16*100, s.Fractions.GT235*100, s.Fractions.EQ16*100, s.Fractions.EQ235*100),
		fmt.Sprintf("黑端锚点：code 0 = %.5f%%；白端锚点：code 255 = %.5f%%", s.Fractions.EQ0*100, s.Fractions.EQ255*100),
	}
}

func defaultCaveats() []string {
	return []string{
		"播放器标签和像素直方图不能证明创作意图；抬黑与风格化暗部仅凭范围分布可能不可区分。",
		"若输入经过转码、去噪、调色或裁切，端点轨道与越界比例会被改变，应以原始母版或带黑/白参考帧的素材复核。",
		"修正标签建议用有限/全范围元数据或解码矩阵再验证；单看 16–235 仍可能是低对比度 full-range 母版。",
	}
}
