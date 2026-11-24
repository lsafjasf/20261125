package audit

import "testing"

func TestAccumulatorStats(t *testing.T) {
	acc := NewAccumulator()
	for i := 0; i < 100; i++ {
		acc.Add(16)
		acc.Add(235)
	}
	acc.Add(100)
	s := acc.Stats(1, 1, 1)
	if s.Samples != 201 {
		t.Fatalf("samples = %d, want 201", s.Samples)
	}
	if s.Min != 16 || s.Max != 235 {
		t.Fatalf("min/max = %d/%d", s.Min, s.Max)
	}
	if s.Counts.EQ16 != 100 || s.Counts.EQ235 != 100 || s.Counts.IN != 201 {
		t.Fatalf("unexpected counts: %+v", s.Counts)
	}
	if s.Percent.P001 != 16 || s.Percent.P50 != 100 || s.Percent.P999 != 235 {
		t.Fatalf("unexpected percentiles: %+v", s.Percent)
	}
	if s.Fractions.Outside != 0 {
		t.Fatalf("outside fraction = %f", s.Fractions.Outside)
	}
}

func TestQuantileNearestRank(t *testing.T) {
	acc := NewAccumulator()
	for i := 0; i <= 100; i++ {
		acc.Add(uint8(i))
	}
	s := acc.Stats(1, 1, 1)
	if s.Percent.P50 != 50 {
		t.Fatalf("p50 = %d, want 50", s.Percent.P50)
	}
	if s.Percent.P95 != 95 {
		t.Fatalf("p95 = %d, want 95", s.Percent.P95)
	}
	if s.Percent.P999 != 100 {
		t.Fatalf("p99.9 = %d, want 100", s.Percent.P999)
	}
}

func TestEmptyStats(t *testing.T) {
	s := NewAccumulator().Stats(0, 0, 0)
	if s.Samples != 0 || s.Min != 0 || s.Max != 0 {
		t.Fatalf("unexpected empty stats: %+v", s)
	}
}
