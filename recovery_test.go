package freezedetect

import (
	"testing"
	"time"
)

// 冻结后时间轴与画面恢复，结论必须能自动退出 Frozen。
func TestRecoveryAfterFreeze(t *testing.T) {
	d := NewDetector(DefaultConfig())
	base := makeStillFrame()
	for i := 0; i < 8; i++ {
		push(t, d, base, nil, 100*time.Millisecond)
	}
	frozen, _ := d.Report(7)
	if frozen.Verdict != VerdictFrozen {
		t.Fatalf("前置条件失败，第 7 帧 = %s", frozen.Verdict)
	}
	recovered := push(t, d, movingFrame(9), nil, 200*time.Millisecond)
	if recovered.TimeStalled {
		t.Fatal("PTS 恢复前进后应解除时间轴停止")
	}
	if recovered.Verdict == VerdictFrozen {
		t.Fatalf("恢复后不应保持 frozen，实际 = %s", recovered.Verdict)
	}
}

func TestNilImageRejected(t *testing.T) {
	d := NewDetector(DefaultConfig())
	if _, err := d.Push(Frame{PTS: time.Millisecond}); err == nil {
		t.Fatal("nil 图像应返回错误")
	}
}

func TestVerdictStrings(t *testing.T) {
	cases := map[Verdict]string{
		VerdictUnknown:     "unknown",
		VerdictNormal:      "normal",
		VerdictStillScene:  "still_scene",
		VerdictTimeStalled: "time_stalled",
		VerdictFrozen:      "frozen",
	}
	for v, want := range cases {
		if v.String() != want {
			t.Fatalf("Verdict(%d).String() = %q, 期望 %q", v, v.String(), want)
		}
	}
}
