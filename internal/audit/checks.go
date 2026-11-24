package audit

import (
	"fmt"
	"math/big"
)

func checkEditStructure(raw []rawEdit, edits []EditReport, movie *MovieInfo, tkhd *TKHDInfo, mdhd *MDHDInfo, trackIndex int, total *big.Int) []Finding {
	var findings []Finding
	path := "/moov/trak/edts/elst"

	if movie == nil || movie.Timescale == 0 {
		findings = append(findings, Finding{
			Severity: "error", Code: "mvhd_timescale_unavailable", TrackIndex: trackIndex,
			BoxPath: "/moov/mvhd", Message: "mvhd.timescale is missing or zero",
		})
	}
	if mdhd == nil || mdhd.Timescale == 0 {
		findings = append(findings, Finding{
			Severity: "error", Code: "mdhd_timescale_unavailable", TrackIndex: trackIndex,
			BoxPath: "/moov/trak/mdia/mdhd", Message: "mdhd.timescale is missing or zero",
		})
	}
	if movie != nil && mdhd != nil && movie.Timescale != mdhd.Timescale {
		findings = append(findings, Finding{
			Severity: "info", Code: "cross_timescale", TrackIndex: trackIndex, BoxPath: path,
			Message: fmt.Sprintf("cross-timescale conversion: mvhd=%d, mdhd=%d", movie.Timescale, mdhd.Timescale),
		})
	}
	if len(raw) == 0 {
		findings = append(findings, Finding{
			Severity: "warning", Code: "empty_elst", TrackIndex: trackIndex, BoxPath: path,
			Message: "elst entry count is zero",
		})
	}
	if len(raw) > 0 && raw[len(raw)-1].mediaTime == -1 {
		findings = append(findings, Finding{
			Severity: "warning", Code: "last_edit_empty", TrackIndex: trackIndex, BoxPath: path,
			EditIndex: len(raw) - 1, Message: "the last explicit edit is empty; specifications recommend against it",
		})
	}
	if tkhd != nil && total != nil && new(big.Int).SetUint64(tkhd.Duration).Cmp(total) != 0 {
		findings = append(findings, Finding{
			Severity: "warning", Code: "tkhd_elst_duration_mismatch", TrackIndex: trackIndex, BoxPath: "/moov/trak/tkhd",
			Message: fmt.Sprintf("tkhd.duration=%d but sum(elst.segment_duration)=%s in mvhd units", tkhd.Duration, total.String()),
		})
	}

	for i, edit := range edits {
		rate := new(big.Rat)
		if _, ok := rate.SetString(edit.RateRational); !ok {
			rate = big.NewRat(1, 1)
		}
		if rate.Sign() < 0 {
			findings = append(findings, Finding{
				Severity: "warning", Code: "negative_media_rate", TrackIndex: trackIndex, EditIndex: i, BoxPath: path,
				Message: "negative media_rate implies reverse playback and is rejected by Apple QTFF guidance",
			})
		}
		if rate.Sign() > 0 && rate.Cmp(big.NewRat(1, 1)) != 0 {
			findings = append(findings, Finding{
				Severity: "info", Code: "non_unit_media_rate", TrackIndex: trackIndex, EditIndex: i, BoxPath: path,
				Message: fmt.Sprintf("non-unit positive media rate %s", edit.RateFixed),
			})
		}
		if raw[i].segmentDuration == 0 && !edit.Empty {
			findings = append(findings, Finding{
				Severity: "info", Code: "zero_segment_duration", TrackIndex: trackIndex, EditIndex: i, BoxPath: path,
				Message: "zero segment duration can encode a priming offset; it occupies no composition interval",
			})
		}
		if edit.Media != nil && mdhd != nil && mdhd.Duration > 0 {
			duration := new(big.Rat).SetInt(new(big.Int).SetUint64(mdhd.Duration))
			for _, unit := range []string{edit.Media.StartUnits, edit.Media.EndUnits} {
				value, ok := new(big.Rat).SetString(unit)
				if !ok {
					continue
				}
				if value.Cmp(duration) > 0 {
					findings = append(findings, Finding{
						Severity: "warning", Code: "edit_beyond_media_duration", TrackIndex: trackIndex, EditIndex: i, BoxPath: path,
						Message: fmt.Sprintf("media time %s exceeds mdhd.duration=%d", unit, mdhd.Duration),
					})
					break
				}
			}
		}
	}

	return findings
}
