package freezedetect

import (
	"fmt"
	"time"
)

// Detector 逐帧评估卡死状态。
//
// 两条独立信号：
//   - PictureStatic：画面层是否静止（像素度量 + 噪声/闪烁证据修正）
//   - TimeStalled：PTS 是否停止前进（与画面内容完全无关）
//
// 综合结论只有在两者同时成立时才判 VerdictFrozen，避免把长静止镜头
// 误报为卡死。
type Detector struct {
	cfg   Config
	index int

	prev        *frameMetrics
	prevSub     Image
	hasSub      bool
	prevPTS     time.Duration
	havePrev    bool
	staticRun   int
	stallRun    int
	flickerLeft int

	reports []FrameReport
}

// NewDetector 以给定配置构造检测器，零值字段用默认值补齐。
func NewDetector(cfg Config) *Detector {
	def := DefaultConfig()
	if cfg.PixelChangeRatio <= 0 {
		cfg.PixelChangeRatio = def.PixelChangeRatio
	}
	if cfg.MeanDiffEpsilon <= 0 {
		cfg.MeanDiffEpsilon = def.MeanDiffEpsilon
	}
	if cfg.DarkLuma == 0 {
		cfg.DarkLuma = def.DarkLuma
	}
	if cfg.DarkRatio <= 0 {
		cfg.DarkRatio = def.DarkRatio
	}
	if cfg.NoiseVarianceMin <= 0 {
		cfg.NoiseVarianceMin = def.NoiseVarianceMin
	}
	if cfg.FlickerDelta <= 0 {
		cfg.FlickerDelta = def.FlickerDelta
	}
	if cfg.FlickerMinConfirms <= 0 {
		cfg.FlickerMinConfirms = def.FlickerMinConfirms
	}
	if cfg.StallMinConfirms <= 0 {
		cfg.StallMinConfirms = def.StallMinConfirms
	}
	if cfg.FreezeMinConfirms <= 0 {
		cfg.FreezeMinConfirms = def.FreezeMinConfirms
	}
	if cfg.StillMinConfirms <= 0 {
		cfg.StillMinConfirms = def.StillMinConfirms
	}
	return &Detector{cfg: cfg}
}

// Reset 清空全部状态与历史报告。
func (d *Detector) Reset() {
	d.prev, d.prevSub, d.hasSub = nil, nil, false
	d.havePrev = false
	d.staticRun, d.stallRun, d.flickerLeft = 0, 0, 0
	d.index = 0
	d.reports = nil
}

// Push 送入一帧并返回该帧报告。
func (d *Detector) Push(f Frame) (FrameReport, error) {
	if f.Img == nil {
		return FrameReport{}, fmt.Errorf("freezedetect: frame %d has nil image", d.index)
	}
	cur := sampleMetrics(f.Img, d.cfg.DarkLuma)
	report := FrameReport{Index: d.index, PTS: f.PTS}

	if !d.havePrev {
		d.prev = &cur
		d.prevSub, d.hasSub = f.Subtitle, f.Subtitle != nil
		d.prevPTS = f.PTS
		d.havePrev = true
		report.Evidence.LumaMean = cur.lumaMean
		report.Evidence.DarkRatio = cur.darkRatio
		report.Verdict = VerdictUnknown
		report.Reasons = []string{"首帧，缺少参照帧"}
		d.reports = append(d.reports, report)
		d.index++
		return report, nil
	}

	dm := compareFrames(*d.prev, cur, d.cfg)
	ev := Evidence{
		MeanDiff:      dm.meanDiff,
		ChangedRatio:  dm.changedRatio,
		DarkRatio:     cur.darkRatio,
		NoiseVariance: dm.noiseVariance,
		LumaMean:      cur.lumaMean,
		LumaDelta:     dm.lumaDelta,
	}

	// 证据 1：低照噪声。暗场静态画面会有传感器残差噪声，
	// 这种像素抖动不是运动，不能据此否定画面静止。
	ev.HasLowLightNoise = cur.darkRatio >= d.cfg.DarkRatio &&
		dm.noiseVariance >= d.cfg.NoiseVarianceMin

	// 证据 2：闪烁。整体亮度跳变说明采集链路仍在输出新像素，
	// 真正的采集冻结不会产生亮度变化。
	flickerNow := abs64(dm.lumaDelta) >= d.cfg.FlickerDelta
	if flickerNow {
		d.flickerLeft = d.cfg.FlickerMinConfirms
	}
	ev.HasFlicker = d.flickerLeft > 0
	if d.flickerLeft > 0 {
		d.flickerLeft--
	}

	// 证据 3：字幕/OSD 层独立比较，字幕变化不等于画面运动。
	if f.Subtitle != nil || d.hasSub {
		ev.SubtitleChanged = !imagesEqual(d.prevSub, f.Subtitle)
	}

	// 画面层静止判断：阈值判定，暗场噪声抖动宽容处理，
	// 但闪烁属于亮度活动，按非静止处理。
	thresholdStatic := dm.changedRatio <= d.cfg.PixelChangeRatio &&
		dm.meanDiff <= d.cfg.MeanDiffEpsilon
	switch {
	case flickerNow:
		ev.PictureStatic = false
	case thresholdStatic:
		ev.PictureStatic = true
	case ev.HasLowLightNoise && dm.meanDiff <= d.cfg.FlickerDelta:
		ev.PictureStatic = true
	default:
		ev.PictureStatic = false
	}

	// 时间轴判断：只看 PTS，与像素内容完全解耦。
	ev.TimeAdvanced = f.PTS > d.prevPTS
	if ev.TimeAdvanced {
		d.stallRun = 0
	} else {
		d.stallRun++
	}
	report.TimeStalled = d.stallRun >= d.cfg.StallMinConfirms

	if ev.PictureStatic {
		d.staticRun++
	} else {
		d.staticRun = 0
	}
	report.PictureStatic = ev.PictureStatic
	report.Evidence = ev
	report.Verdict, report.Reasons = d.decide(ev)

	d.prev = &cur
	d.prevSub, d.hasSub = f.Subtitle, f.Subtitle != nil
	d.prevPTS = f.PTS
	d.reports = append(d.reports, report)
	d.index++
	return report, nil
}

func (d *Detector) decide(ev Evidence) (Verdict, []string) {
	reasons := make([]string, 0, 5)
	if ev.PictureStatic {
		reasons = append(reasons, "画面层静止")
	} else {
		reasons = append(reasons, "画面层存在变化")
	}
	if ev.HasLowLightNoise {
		reasons = append(reasons, "观察到低照传感器噪声")
	}
	if ev.HasFlicker {
		reasons = append(reasons, "观察到亮度闪烁")
	}
	if ev.SubtitleChanged {
		reasons = append(reasons, "字幕/OSD 层发生变化")
	}
	if ev.TimeAdvanced {
		reasons = append(reasons, "PTS 正常前进")
	} else {
		reasons = append(reasons, "PTS 未前进")
	}

	staticConfirmed := d.staticRun >= d.cfg.FreezeMinConfirms
	switch {
	case reportStalled(d) && staticConfirmed && !ev.SubtitleChanged:
		reasons = append(reasons, "画面静止且时间轴停止，判定卡死")
		return VerdictFrozen, reasons
	case reportStalled(d):
		reasons = append(reasons, "时间轴停止但画面/字幕仍有活动，判定时间戳停滞")
		return VerdictTimeStalled, reasons
	case ev.PictureStatic && d.staticRun >= d.cfg.StillMinConfirms:
		reasons = append(reasons, "时间轴前进而画面静止，判定长静止镜头")
		return VerdictStillScene, reasons
	default:
		return VerdictNormal, reasons
	}
}

func reportStalled(d *Detector) bool {
	return d.stallRun >= d.cfg.StallMinConfirms
}

// Override 人工修正指定帧的结论。自动判定状态不依赖人工结论，
// 因此修正单帧不会影响其他帧的自动结果。
func (d *Detector) Override(index int, verdict Verdict) error {
	if index < 0 || index >= len(d.reports) {
		return fmt.Errorf("freezedetect: frame index %d 不存在", index)
	}
	old := d.reports[index].Verdict
	d.reports[index].Verdict = verdict
	d.reports[index].Overridden = true
	d.reports[index].Reasons = append(
		d.reports[index].Reasons,
		fmt.Sprintf("人工修正: %s -> %s", old, verdict),
	)
	return nil
}

// Report 返回指定帧的报告副本。
func (d *Detector) Report(index int) (FrameReport, error) {
	if index < 0 || index >= len(d.reports) {
		return FrameReport{}, fmt.Errorf("freezedetect: frame index %d 不存在", index)
	}
	return d.reports[index], nil
}

// Reports 返回全部历史报告副本。
func (d *Detector) Reports() []FrameReport {
	out := make([]FrameReport, len(d.reports))
	copy(out, d.reports)
	return out
}

// StillFrames 返回当前连续静止帧计数。
func (d *Detector) StillFrames() int {
	return d.staticRun
}

func abs64(v float64) float64 {
	if v < 0 {
		return -v
	}
	return v
}
