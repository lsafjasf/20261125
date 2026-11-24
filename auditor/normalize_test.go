package auditor

import (
	"math"
	"testing"
)

func TestNormalizeLanguage(t *testing.T) {
	cases := []struct {
		raw     string
		want    string
		wantErr bool
	}{
		{"en", "en", false},
		{"EN", "en", false},
		{"en-US", "en", false},
		{"zh_CN", "zh", false},
		{"eng", "en", false},
		{"chi", "zh", false},
		{"jpn", "ja", false},
		{"", "", true},
		{"123", "", true},
	}
	for _, tc := range cases {
		got, err := NormalizeLanguage(tc.raw)
		if tc.wantErr {
			if err == nil {
				t.Errorf("NormalizeLanguage(%q) expected error, got %q", tc.raw, got)
			}
			continue
		}
		if err != nil {
			t.Errorf("NormalizeLanguage(%q) unexpected error: %v", tc.raw, err)
		}
		if got != tc.want {
			t.Errorf("NormalizeLanguage(%q) = %q, want %q", tc.raw, got, tc.want)
		}
	}
}

func TestParseDuration(t *testing.T) {
	cases := []struct {
		raw     string
		want    float64
		wantErr bool
	}{
		{"6123.5", 6123.5, false},
		{"01:42:03.5", 6123.5, false},
		{"42:03.5", 2523.5, false},
		{"1h2m3.5s", 3723.5, false},
		{"10.5s", 10.5, false},
		{"", 0, true},
		{"not-a-duration", 0, true},
		{"-5", 0, true},
	}
	for _, tc := range cases {
		got, err := ParseDuration(tc.raw)
		if tc.wantErr {
			if err == nil {
				t.Errorf("ParseDuration(%q) expected error, got %v", tc.raw, got)
			}
			continue
		}
		if err != nil {
			t.Errorf("ParseDuration(%q) unexpected error: %v", tc.raw, err)
		}
		if math.Abs(got-tc.want) > 1e-9 {
			t.Errorf("ParseDuration(%q) = %v, want %v", tc.raw, got, tc.want)
		}
	}
}

func TestCanonicalDuration(t *testing.T) {
	if got := CanonicalDuration(6123.5006); got != "6123.501s" {
		t.Fatalf("CanonicalDuration rounding = %q", got)
	}
}
