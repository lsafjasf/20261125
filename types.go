package freezedetect

import "time"

// Verdict 是单帧结论。视觉状态与时间轴状态始终分开评估，
// Verdict 是两者的综合判定结果。
type Verdict int

const (
	// VerdictUnknown 尚未有足够信息（例如第一帧）。
	VerdictUnknown Verdict = iota
	// VerdictNormal 画面在变化，时间轴在前进。
	VerdictNormal
	// VerdictStillScene 画面静止但时间轴在前进，属于长静止镜头。
	VerdictStillScene
	// VerdictTimeStalled 画面在变化但时间轴不前进，属于时间戳异常。
	VerdictTimeStalled
	// VerdictFrozen 画面不变且时间轴不前进，判定为采集/播放卡死。
	VerdictFrozen
)

func (v Verdict) String() string {
	switch v {
	case VerdictNormal:
		return "normal"
	case VerdictStillScene:
		return "still_scene"
	case VerdictTimeStalled:
		return "time_stalled"
	case VerdictFrozen:
		return "frozen"
	default:
		return "unknown"
	}
}

// Config 控制检测器灵敏度。零值字段会在 NewDetector 中替换为默认值。
type Config struct {
	// PixelChangeRatio 相邻帧像素差异比例超过该值即认为画面发生变化。
	PixelChangeRatio float64
	// MeanDiffEpsilon 相邻帧平均灰度差低于该值视为接近一致。
	MeanDiffEpsilon float64
	// DarkLuma 灰度低于该值的像素计入低照区域。
	DarkLuma uint8
	// DarkRatio 低照像素占比超过该值时启用噪声证据。
	DarkRatio float64
	// NoiseVarianceMin 低照区域帧间方差超过该值视为存在传感器噪声。
	NoiseVarianceMin float64
	// FlickerDelta 帧间整体平均亮度变化超过该值视为闪烁。
	FlickerDelta float64
	// FlickerMinConfirms 检测到闪烁后，至少多少帧内维持闪烁证据。
	FlickerMinConfirms int
	// StallMinConfirms 时间戳连续不前进多少帧才确认时间轴停止。
	StallMinConfirms int
	// FreezeMinConfirms 画面连续静止多少帧且时间轴停止才确认卡死。
	FreezeMinConfirms int
	// StillMinConfirms 画面连续静止多少帧（时间轴正常）才标记长静止镜头。
	StillMinConfirms int
}

// DefaultConfig 返回面向常见视频帧尺寸的默认阈值。
func DefaultConfig() Config {
	return Config{
		PixelChangeRatio:   0.002,
		MeanDiffEpsilon:    1.5,
		DarkLuma:           32,
		DarkRatio:          0.6,
		NoiseVarianceMin:   1.2,
		FlickerDelta:       4.0,
		FlickerMinConfirms: 3,
		StallMinConfirms:   3,
		FreezeMinConfirms:  3,
		StillMinConfirms:   3,
	}
}

// Frame 是喂给检测器的一帧。
//
// Img 为完整画面；Subtitle 可选，是字幕层（例如 OSD 渲染层或字幕区域
// 裁剪图）。字幕层独立比较，避免字幕变化把静止镜头判成动态画面。
// PTS 是该帧的呈现时间戳，必须单调（相等表示时间轴没有前进）。
type Frame struct {
	Img      Image
	Subtitle Image
	PTS      time.Duration
}

// Evidence 记录一帧相对上一帧的全部原始证据，结论与证据解耦，
// 便于事后审计和人工修正。
type Evidence struct {
	// MeanDiff 画面平均灰度差。
	MeanDiff float64
	// ChangedRatio 发生变化的像素占比。
	ChangedRatio float64
	// DarkRatio 低照像素占比。
	DarkRatio float64
	// NoiseVariance 低照区域帧间残差方差；非低照场景为 0。
	NoiseVariance float64
	// LumaMean 当前帧平均亮度。
	LumaMean float64
	// LumaDelta 相对上一帧的平均亮度变化（带符号）。
	LumaDelta float64
	// PictureStatic 画面层是否静止。
	PictureStatic bool
	// SubtitleChanged 字幕层是否发生变化。
	SubtitleChanged bool
	// HasLowLightNoise 是否观察到低照噪声证据。
	HasLowLightNoise bool
	// HasFlicker 是否观察到闪烁证据（含确认窗口内的持续标记）。
	HasFlicker bool
	// TimeAdvanced 时间戳相对上一帧是否前进。
	TimeAdvanced bool
}

// FrameReport 是单帧检测输出。
type FrameReport struct {
	// Index 帧序号，从 0 开始。
	Index int
	PTS   time.Duration
	// Verdict 综合结论。
	Verdict Verdict
	// PictureStatic 与时间轴无关的画面静止判断。
	PictureStatic bool
	// TimeStalled 与画面无关的时间轴停止判断。
	TimeStalled bool
	// Evidence 原始度量证据。
	Evidence Evidence
	// Overridden 为 true 表示结论来自人工修正而非自动判定。
	Overridden bool
	// Reasons 人类可读的判定依据。
	Reasons []string
}
