package rational

import "testing"

func TestNormalize(t *testing.T) {
	r := New(30000, 1001)
	if r.Num != 30000 || r.Den != 1001 {
		t.Fatalf("got %v", r)
	}
	r = New(4, 8)
	if r.Num != 1 || r.Den != 2 {
		t.Fatalf("got %v", r)
	}
	r = New(1, -2)
	if r.Num != -1 || r.Den != 2 {
		t.Fatalf("sign normalization failed: %v", r)
	}
}

func TestArithmetic(t *testing.T) {
	a := New(1, 3)
	b := New(1, 6)
	if got := a.Add(b); got != New(1, 2) {
		t.Fatalf("add: %v", got)
	}
	if got := a.Sub(b); got != New(1, 6) {
		t.Fatalf("sub: %v", got)
	}
	if got := a.Mul(b); got != New(1, 18) {
		t.Fatalf("mul: %v", got)
	}
	if got := a.Div(b); got != New(2, 1) {
		t.Fatalf("div: %v", got)
	}
}

func TestRoundInt64(t *testing.T) {
	cases := []struct {
		r    Rat
		want int64
	}{
		{New(75075, 100), 751}, // 750.75 -> 751
		{New(15015, 10), 1502}, // 1501.5 -> 1502 (half away from zero)
		{New(-15015, 10), -1502},
		{New(72072, 10), 7207}, // 7207.2 -> 7207
	}
	for _, c := range cases {
		if got := c.r.RoundInt64(); got != c.want {
			t.Errorf("RoundInt64(%v) = %d, want %d", c.r, got, c.want)
		}
	}
}

func TestFrameDriftIsExact(t *testing.T) {
	// 7200 frames at 30000/1001 fps, misread at 30 fps: exact 7207.2 frames.
	sec := New(7200*1001, 30000)
	drift := sec.Mul(New(30, 1))
	if drift != New(36036, 5) {
		t.Fatalf("got %v", drift)
	}
	if drift.RoundInt64() != 7207 {
		t.Fatalf("round: %d", drift.RoundInt64())
	}
}
