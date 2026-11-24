// Package timecode converts between frame counts and SMPTE timecode
// strings, including NTSC drop-frame for 29.97 fps timelines.
package timecode

import "fmt"

// Format renders a frame count as HH:MM:SS:FF (non-drop) or HH:MM:SS;FF
// (drop-frame). nominalFPS is the integer timecode base (30 for 29.97).
func Format(frames int64, nominalFPS int64, drop bool) string {
	if frames < 0 {
		return "-" + Format(-frames, nominalFPS, drop)
	}
	if drop {
		frames = toDropNumbering(frames, nominalFPS)
	}
	fps := nominalFPS
	ff := frames % fps
	total := frames / fps
	ss := total % 60
	mm := (total / 60) % 60
	hh := total / 3600
	sep := ":"
	if drop {
		sep = ";"
	}
	return fmt.Sprintf("%02d:%02d:%02d%s%02d", hh, mm, ss, sep, ff)
}

// Parse converts a timecode string back to a frame count. The separator
// before the frame field selects drop (";" or ".") vs non-drop (":").
func Parse(tc string, nominalFPS int64) (int64, error) {
	var hh, mm, ss, ff int64
	var sep rune
	n, err := fmt.Sscanf(tc, "%02d:%02d:%02d%c%02d", &hh, &mm, &ss, &sep, &ff)
	if n != 5 || err != nil {
		return 0, fmt.Errorf("timecode: cannot parse %q", tc)
	}
	if mm > 59 || ss > 59 || ff >= nominalFPS {
		return 0, fmt.Errorf("timecode: field out of range in %q", tc)
	}
	frames := ((hh*60+mm)*60+ss)*nominalFPS + ff
	if sep == ';' || sep == '.' {
		drop := nominalFPS / 15
		if ss == 0 && mm%10 != 0 && ff < drop {
			return 0, fmt.Errorf("timecode: %q is a dropped label", tc)
		}
		frames = fromDropNumbering(frames, nominalFPS)
	} else if sep != ':' {
		return 0, fmt.Errorf("timecode: bad separator in %q", tc)
	}
	return frames, nil
}

// toDropNumbering converts an actual frame count to the frame number that
// drop-frame timecode assigns to it (dropping 2 labels per minute, except
// every 10th minute, for a 30 fps nominal base).
func toDropNumbering(frames, nominalFPS int64) int64 {
	drop := nominalFPS / 15 // 2 for 30 fps nominal
	perMin := nominalFPS * 60
	per10Min := perMin*10 - drop*9
	tens := frames / per10Min
	rem := frames % per10Min
	// First minute of each 10-minute block drops nothing.
	if rem < perMin {
		return frames + drop*9*tens
	}
	rem -= perMin
	units := rem / (perMin - drop)
	return frames + drop*9*tens + drop*units + drop
}

// fromDropNumbering converts a drop-frame label back to the actual frame
// count (inverse of toDropNumbering).
func fromDropNumbering(label, nominalFPS int64) int64 {
	drop := nominalFPS / 15
	perMin := nominalFPS * 60
	per10MinLabels := perMin * 10
	tens := label / per10MinLabels
	rem := label % per10MinLabels
	// rem is a label within a 10-minute block; minute 0 has no drops.
	if rem < perMin {
		return label - drop*9*tens
	}
	rem -= perMin
	units := rem / perMin
	return label - drop*9*tens - drop*units - drop
}
