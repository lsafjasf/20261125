package freezedetect

import (
	"math/rand"
	"testing"
	"time"
)

func push(t *testing.T, d *Detector, img Image, sub Image, pts time.Duration) FrameReport {
	t.Helper()
	r, err := d.Push(Frame{Img: img, Subtitle: sub, PTS: pts})
	if err != nil {
		t.Fatalf("Push 失败: %v", err)
	}
	return r
}

// 场景 1：正常静物长镜头。画面完全不变，PTS 持续前进，
// 必须判 StillScene，绝不能判 Frozen。
func TestNormalStillLifeNotFrozen(t *testing.T) {
	d := NewDetector(DefaultConfig())
	base := makeStillFrame()
	var last FrameReport
	for i := 0; i < 10; i++ {
		last = push(t, d, base, nil, time.Duration(i)*33*time.Millisecond)
	}
	if last.Verdict != VerdictStillScene {
		t.Fatalf("静物长镜头结论 = %s, 期望 still_scene", last.Verdict)
	}
	if last.TimeStalled {
		t.Fatal("静物镜头时间轴不应判定停止")
	}
	if !last.PictureStatic {
		t.Fatal("完全一致的画面应判定静止")
	}
}

// 场景 2：静态编码。静物叠加编码器轻微量化抖动（少量像素 1-2 级灰度差），
// 内容不变、PTS 前进，应判 StillScene 而非卡死或动态。
func TestStaticEncodingJitter(t *testing.T) {
	d := NewDetector(DefaultConfig())
	rng := rand.New(rand.NewSource(7))
	base := makeStillFrame()
	var last FrameReport
	for i := 0; i < 10; i++ {
		frame := addNoise(base, rng, 1) // 每像素 ±1 灰度级，模拟编码量化
		last = push(t, d, frame, nil, time.Duration(i)*40*time.Millisecond)
	}
	if last.Verdict != VerdictStillScene {
		t.Fatalf("静态编码结论 = %s, 期望 still_scene", last.Verdict)
	}
	if last.TimeStalled {
		t.Fatal("编码抖动场景时间轴正常")
	}
}

// 场景 3：采集冻结。完全相同的帧重复，PTS 也不前进，
// 必须在确认窗口后判 Frozen。
func TestCaptureFreeze(t *testing.T) {
	d := NewDetector(DefaultConfig())
	base := makeStillFrame()
	frozenPTS := 100 * time.Millisecond
	var verdicts []Verdict
	for i := 0; i < 8; i++ {
		r := push(t, d, base, nil, frozenPTS)
		verdicts = append(verdicts, r.Verdict)
	}
	if verdicts[len(verdicts)-1] != VerdictFrozen {
		t.Fatalf("采集冻结结论 = %s, 期望 frozen", verdicts[len(verdicts)-1])
	}
	var sawFrozen bool
	for _, v := range verdicts {
		if v == VerdictFrozen {
			sawFrozen = true
		}
	}
	if !sawFrozen {
		t.Fatal("确认窗口内应出现 frozen 结论")
	}
}

// 场景 4：低照噪声。暗场静物每帧带传感器噪声，PTS 前进，
// 噪声作为证据记录，但画面仍判静止、结论为 StillScene。
func TestLowLightNoiseStillScene(t *testing.T) {
	d := NewDetector(DefaultConfig())
	rng := rand.New(rand.NewSource(11))
	base := makeDarkFrame()
	var last FrameReport
	for i := 0; i < 10; i++ {
		frame := addNoise(base, rng, 4)
		last = push(t, d, frame, nil, time.Duration(i)*33*time.Millisecond)
	}
	if !last.Evidence.HasLowLightNoise {
		t.Fatalf("暗场噪声方差 = %.2f, 应记录低照噪声证据", last.Evidence.NoiseVariance)
	}
	if !last.PictureStatic {
		t.Fatal("低照噪声抖动不应否定画面静止")
	}
	if last.Verdict != VerdictStillScene {
		t.Fatalf("低照静物结论 = %s, 期望 still_scene", last.Verdict)
	}
}

// 场景 5：字幕变化。画面静止、PTS 前进，只有字幕层变化，
// 画面层仍判静止且结论为 StillScene，字幕证据为 true。
func TestSubtitleChangeKeepsPictureStatic(t *testing.T) {
	d := NewDetector(DefaultConfig())
	base := makeStillFrame()
	var last FrameReport
	for i := 0; i < 8; i++ {
		sub := subtitleBar(i)
		last = push(t, d, base, sub, time.Duration(i)*33*time.Millisecond)
	}
	if !last.Evidence.SubtitleChanged {
		t.Fatal("字幕 ID 变化应记录字幕变化证据")
	}
	if !last.PictureStatic {
		t.Fatal("字幕变化不应让画面层判定为动态")
	}
	if last.Verdict != VerdictStillScene {
		t.Fatalf("字幕切换下静物结论 = %s, 期望 still_scene", last.Verdict)
	}
}

// 场景 6：闪烁。亮度整体跳变且 PTS 停止时，闪烁证明采集仍在输出，
// 结论应为 TimeStalled 而不是 Frozen。
func TestFlickerPreventsFalseFreeze(t *testing.T) {
	d := NewDetector(DefaultConfig())
	base := makeStillFrame()
	frozenPTS := 200 * time.Millisecond
	var last FrameReport
	for i := 0; i < 8; i++ {
		var frame Image
		if i%2 == 0 {
			frame = shiftLuma(base, 12)
		} else {
			frame = shiftLuma(base, -12)
		}
		last = push(t, d, frame, nil, frozenPTS)
	}
	if !last.TimeStalled {
		t.Fatal("PTS 不前进应确认时间轴停止")
	}
	if !last.Evidence.HasFlicker {
		t.Fatal("亮度交替跳变应记录闪烁证据")
	}
	if last.Verdict == VerdictFrozen {
		t.Fatalf("闪烁场景不应判卡死，实际 = %s", last.Verdict)
	}
	if last.Verdict != VerdictTimeStalled {
		t.Fatalf("闪烁+PTS 停滞结论 = %s, 期望 time_stalled", last.Verdict)
	}
}

// 场景 7：画面在变但 PTS 停止，独立验证时间轴通道，判 TimeStalled。
func TestTimeStalledWithMovingPicture(t *testing.T) {
	d := NewDetector(DefaultConfig())
	var last FrameReport
	for i := 0; i < 8; i++ {
		last = push(t, d, movingFrame(i), nil, 50*time.Millisecond)
	}
	if last.PictureStatic {
		t.Fatal("运动画面不应判静止")
	}
	if !last.TimeStalled {
		t.Fatal("PTS 恒定应判时间轴停止")
	}
	if last.Verdict != VerdictTimeStalled {
		t.Fatalf("运动+PTS 停滞结论 = %s, 期望 time_stalled", last.Verdict)
	}
}

// 场景 8：人工修正。自动判定的单帧结论可被覆盖，且不影响其他帧。
func TestManualOverrideSingleFrame(t *testing.T) {
	d := NewDetector(DefaultConfig())
	base := makeStillFrame()
	for i := 0; i < 8; i++ {
		push(t, d, base, nil, time.Duration(i)*33*time.Millisecond)
	}
	target := 5
	if err := d.Override(target, VerdictFrozen); err != nil {
		t.Fatalf("Override 失败: %v", err)
	}
	r5, _ := d.Report(target)
	if r5.Verdict != VerdictFrozen || !r5.Overridden {
		t.Fatalf("第 %d 帧应已人工修正为 frozen", target)
	}
	r4, _ := d.Report(target - 1)
	if r4.Overridden || r4.Verdict != VerdictStillScene {
		t.Fatal("人工修正单帧不应影响相邻帧的自动结论")
	}
	r6, _ := d.Report(target + 1)
	if r6.Overridden || r6.Verdict != VerdictStillScene {
		t.Fatal("人工修正单帧不应影响相邻帧的自动结论")
	}
	if err := d.Override(999, VerdictNormal); err == nil {
		t.Fatal("越界帧号应返回错误")
	}
}
