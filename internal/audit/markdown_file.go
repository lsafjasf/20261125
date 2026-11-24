package audit

import (
	"fmt"
	"strings"
)

func renderFile(b *strings.Builder, file *FileReport, index int) {
	b.WriteString(fmt.Sprintf("## %d. `%s`\n\n", index+1, file.Path))
	b.WriteString(fmt.Sprintf("- 文件大小：%d bytes\n", file.FileSize))
	if file.Error != "" {
		b.WriteString("- 解析错误：`" + file.Error + "`\n\n")
	}
	if file.Movie != nil {
		b.WriteString(fmt.Sprintf("- `mvhd`：timescale=`%d`，duration=`%d`（%s s）\n",
			file.Movie.Timescale, file.Movie.Duration, file.Movie.DurationSecond))
	}
	b.WriteString("\n")

	for trackIndex := range file.Tracks {
		renderTrack(b, file, &file.Tracks[trackIndex])
	}
	renderFindings(b, file.Findings)
}

func renderTrack(b *strings.Builder, file *FileReport, track *TrackReport) {
	title := fmt.Sprintf("### trak #%d", track.Index+1)
	if track.TrackID != 0 {
		title += fmt.Sprintf(" / track_id=%d", track.TrackID)
	}
	if track.Handler != "" {
		title += " / handler=`" + track.Handler + "`"
	}
	if track.IsAudio {
		title += " / 音频轨"
	}
	b.WriteString(title + "\n\n")

	b.WriteString("| 层 | 原始字段 | 值 | 时间换算 |\n")
	b.WriteString("|---|---|---:|---|\n")
	if file.Movie != nil {
		b.WriteString(fmt.Sprintf("| `mvhd` | timescale / duration | `%d` / `%d` | duration `%s s` |\n",
			file.Movie.Timescale, file.Movie.Duration, file.Movie.DurationSecond))
	}
	if track.TKHD != nil {
		seconds := "undefined"
		if file.Movie != nil && file.Movie.Timescale != 0 {
			seconds = rationalSeconds(track.TKHD.Duration, file.Movie.Timescale) + " s"
		}
		b.WriteString(fmt.Sprintf("| `trak/tkhd` | version / track_id / duration | `%d` / `%d` / `%d` | tkhd duration 与 elst segment 同属 mvhd timescale：`%s` |\n",
			track.TKHD.Version, track.TKHD.TrackID, track.TKHD.Duration, seconds))
	}
	if track.ELST != nil {
		b.WriteString(fmt.Sprintf("| `trak/edts/elst` | offset / version / entry_count | `%d` / `%d` / `%d` | segment_duration 使用 mvhd timescale |\n",
			track.ELST.Offset, track.ELST.Version, track.ELST.EntryCount))
	} else {
		b.WriteString("| `trak/edts` | elst | 缺失 | 无编辑列表时从 media time 0、composition 0 开始 |\n")
	}
	if track.MDHD != nil {
		b.WriteString(fmt.Sprintf("| `mdia/mdhd` | version / timescale / duration | `%d` / `%d` / `%d` | duration `%s s`；media_time 使用此 timescale |\n\n",
			track.MDHD.Version, track.MDHD.Timescale, track.MDHD.Duration,
			rationalSeconds(track.MDHD.Duration, track.MDHD.Timescale)))
	}

	renderAudioStart(b, track.AudioStart)
	renderEdits(b, track.Edits)
}

func renderAudioStart(b *strings.Builder, start *AudioStart) {
	if start == nil {
		b.WriteString("**音频起点**：没有非 empty edit，无法确定声源起点。\n\n")
		return
	}
	b.WriteString("**音频实际起点判定**\n\n")
	b.WriteString(fmt.Sprintf("- 首个有声/驻留 edit：`#%d`（`%s`）\n", start.EditIndex, start.Kind))
	b.WriteString(fmt.Sprintf("- 源媒体起点 `media_time`：`%d` mdia units = `%s s`\n",
		start.SourceMediaTimeUnits, start.SourceMediaTimeSeconds))
	b.WriteString(fmt.Sprintf("- 合成起点：`%s` mvhd units = `%s s`\n",
		start.CompositionStartUnits, start.CompositionStartSeconds))
	if start.LeadingEmptySeconds != "" && start.LeadingEmptySeconds != "0" {
		b.WriteString(fmt.Sprintf("- 开头 empty edit 延迟：`%s s`；这是实际可闻起点相对时间线 0 的后移量。\n", start.LeadingEmptySeconds))
	}
	if p := start.MediaZeroCompUnits; p != nil {
		b.WriteString(fmt.Sprintf("- 媒体零点映射到合成时间：`%s` mvhd units = `%s s`（float `%.6f`）\n",
			p.UnitsExact, p.SecondsExact, p.Seconds))
		if start.NegativeTimelineOrigin {
			b.WriteString("- **负起点**：媒体零点落在合成时间 0 之前，样本时间戳需要负偏移；edit anchor 本身仍位于上述合成起点。\n")
		}
	}
	if p := start.UnitRateOffsetMedia; p != nil {
		b.WriteString(fmt.Sprintf("- 单位速率时间戳偏移 `T-C×D/M`：`%s` mdia units = `%s s`；负值对应 empty duration 换算后大于 `media_time` 的 FFmpeg `time_offset` 口径。\n",
			p.UnitsExact, p.SecondsExact))
	}
	b.WriteString("\n")
}

func renderEdits(b *strings.Builder, edits []EditReport) {
	if len(edits) == 0 {
		return
	}
	b.WriteString("**每个 edit 的媒体区间与合成时间**\n\n")
	b.WriteString("| edit | 合成区间（mvhd timescale） | 媒体区间（mdia timescale） | media rate 与取整 | 判定/告警 |\n")
	b.WriteString("|---:|---|---|---|---|\n")
	for _, edit := range edits {
		kind := fmt.Sprintf("#%d `%s`", edit.Index, edit.Kind)
		b.WriteString("| " + kind + " | " + intervalCell(&edit.Composition) + " | " + mediaCell(edit.Media) + " | " + rateCell(edit) + " | " + escapeCell(strings.Join(edit.Warnings, "；")) + " |\n")
	}
	b.WriteString("\n")
}

func intervalCell(in *Interval) string {
	if in == nil {
		return "—"
	}
	return br(
		fmt.Sprintf("units `[%s, %s)`", in.StartUnits, in.EndUnits),
		fmt.Sprintf("duration `%s`", in.DurationUnits),
		fmt.Sprintf("seconds `[%s, %s)`", in.StartSeconds, in.EndSeconds),
		fmt.Sprintf("float `[%.6f, %.6f)`", in.StartSecondsF, in.EndSecondsF),
	)
}

func mediaCell(media *Interval) string {
	if media == nil {
		return "无媒体区间（empty edit）"
	}
	return intervalCell(media)
}

func rateCell(edit EditReport) string {
	lines := []string{fmt.Sprintf("raw `%s`", edit.RateRaw), fmt.Sprintf("r=`%s`（%s）", edit.RateRational, edit.RateFixed)}
	if edit.MediaDurationRound != nil {
		r := edit.MediaDurationRound
		lines = append(lines, fmt.Sprintf("duration floor/near/ceil: `%s/%s/%s`", r.Floor, r.Nearest, r.Ceil))
	}
	if p := edit.MediaOriginOnComposition; p != nil {
		lines = append(lines, fmt.Sprintf("media0→comp `%s` M units / `%s s`", p.UnitsExact, p.SecondsExact))
	}
	return br(lines...)
}

func renderFindings(b *strings.Builder, findings []Finding) {
	if len(findings) == 0 {
		b.WriteString("检查项：未发现结构或换算告警。\n\n")
		return
	}
	b.WriteString("**结构与兼容性检查**\n\n")
	b.WriteString("| 级别 | code | 位置 | 信息 |\n")
	b.WriteString("|---|---|---|---|\n")
	for _, f := range findings {
		location := f.BoxPath
		if f.TrackIndex != 0 || f.EditIndex != 0 {
			location += fmt.Sprintf(" (trak=#%d edit=#%d)", f.TrackIndex+1, f.EditIndex)
		}
		b.WriteString("| `" + f.Severity + "` | `" + f.Code + "` | `" + escapeCell(location) + "` | " + escapeCell(f.Message) + " |\n")
	}
	b.WriteString("\n")
}

func escapeCell(s string) string {
	s = strings.ReplaceAll(s, "|", "\\|")
	s = strings.ReplaceAll(s, "\n", " ")
	return s
}
