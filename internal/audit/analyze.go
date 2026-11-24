package audit

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"math"
	"os"
	"strings"
	"time"

	"cutmux/internal/media"
)

const Version = "1.0"

func Run(pathA, pathB string, opts Options) (*Report, error) {
	if opts.LabelA == "" {
		opts.LabelA = "A"
	}
	if opts.LabelB == "" {
		opts.LabelB = "B"
	}
	if opts.Now.IsZero() {
		opts.Now = time.Now()
	}
	if opts.ToleranceMS <= 0 {
		opts.ToleranceMS = 5
	}
	fa, err := media.ReadFile(pathA)
	if err != nil {
		return nil, err
	}
	fb, err := media.ReadFile(pathB)
	if err != nil {
		return nil, err
	}
	sumA, err := fileSHA256(pathA)
	if err != nil {
		return nil, err
	}
	sumB, err := fileSHA256(pathB)
	if err != nil {
		return nil, err
	}
	va, vb := fa.Primary(media.KindVideo), fb.Primary(media.KindVideo)
	aa, ab := fa.Primary(media.KindAudio), fb.Primary(media.KindAudio)
	sa, sb := fa.Primary(media.KindSubtitle), fb.Primary(media.KindSubtitle)
	evA, err := loadFrameEvidence(opts.FramesA, va)
	if err != nil {
		return nil, fmt.Errorf("A frame evidence: %w", err)
	}
	evB, err := loadFrameEvidence(opts.FramesB, vb)
	if err != nil {
		return nil, fmt.Errorf("B frame evidence: %w", err)
	}
	rep := &Report{
		Tool: "cutmux", Version: Version,
		GeneratedUTC: opts.Now.UTC().Format(time.RFC3339),
		LabelA:       opts.LabelA, LabelB: opts.LabelB,
		ContainerA: summarizeFile(fa, sumA),
		ContainerB: summarizeFile(fb, sumB),
	}
	rep.Video, rep.BlackFrames = compareVideo(va, vb, evA, evB, aa, ab, opts)
	rep.Audio = compareAudio(aa, ab, opts)
	rep.Subtitle = compareSubtitle(sa, sb, opts)
	rep.TotalDuration = durationPair(fa, fb)
	rep.Findings = buildFindings(rep, opts)
	rep.Verdict = decideVerdict(rep)
	rep.NextCheck = nextCheck(rep)
	return rep, nil
}

func fileSHA256(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func summarizeFile(f *media.File, digest string) ContainerSummary {
	out := ContainerSummary{
		Path: f.Path, SHA256: digest, Format: string(f.Format), Brand: f.Brand,
		DeclaredMs: ms(f.DeclaredDuration), MeasuredEndMs: ms(f.MaxPresentedEnd()),
		TrackCount: len(f.Tracks), Warnings: append([]string(nil), f.Warnings...),
	}
	for i := range f.Tracks {
		out.Tracks = append(out.Tracks, trackSummary(&f.Tracks[i]))
	}
	return out
}

func durationPair(a, b *media.File) DurationPair {
	return DurationPair{
		A:     DurationBreakdown{DeclaredMs: ms(a.DeclaredDuration), MeasuredMs: ms(a.MaxPresentedEnd())},
		B:     DurationBreakdown{DeclaredMs: ms(b.DeclaredDuration), MeasuredMs: ms(b.MaxPresentedEnd())},
		Delta: ms(a.MaxPresentedEnd()) - ms(b.MaxPresentedEnd()),
	}
}

func cutPoint(t *media.Track, skipEmpty bool) CutPoint {
	if t == nil {
		return CutPoint{}
	}
	ps := t.PresentedSamples()
	var first, last *media.PresentedSample
	for i := range ps {
		s := &ps[i]
		if skipEmpty {
			if s.DecodeIndex < 0 || s.DecodeIndex >= len(t.Samples) || t.Samples[s.DecodeIndex].Empty {
				continue
			}
		}
		if first == nil {
			first = s
		}
		last = s
	}
	if first == nil || last == nil {
		return CutPoint{}
	}
	return CutPoint{
		StartMs: ms(first.Start), EndMs: ms(last.End), Samples: len(ps),
		FirstHash: fmt.Sprintf("%016x", first.Hash),
		LastHash:  fmt.Sprintf("%016x", last.Hash),
	}
}

func timelinePair(a, b *media.Track, skipEmpty bool) TimelinePair {
	ca, cb := cutPoint(a, skipEmpty), cutPoint(b, skipEmpty)
	return TimelinePair{A: ca, B: cb, Delta: ca.StartMs - cb.StartMs}
}

func compareVideo(va, vb *media.Track, evA, evB frameEvidence, aa, ab *media.Track, opts Options) (VideoComparison, BlackComparison) {
	out := VideoComparison{
		Timeline: timelinePair(va, vb, false),
	}
	bc := BlackComparison{A: evA.Summary, B: evB.Summary}
	if va == nil || vb == nil {
		out.Classification = "missing-video-track"
		out.Explanation = "至少一侧没有视频轨"
		bc.Classification = "not-applicable"
		return out, bc
	}
	ra, rb := va.FrameRate(), vb.FrameRate()
	out.FPSA, out.FPSB = ra.String(), rb.String()
	out.FPSMatch = media.FrameRateEqual(ra, rb)
	out.FrameDeltaMs = math.Abs(float64(va.FrameDurationNanos()-vb.FrameDurationNanos())) / 1e6
	out.RawFramesA, out.RawFramesB = len(va.Samples), len(vb.Samples)
	psa, psb := va.PresentedSamples(), vb.PresentedSamples()
	out.PresentedA, out.PresentedB = len(psa), len(psb)
	al, onlyA, onlyB := alignTracks(va, vb)
	out.Identity, out.OnlyA, out.OnlyB = al, onlyA, onlyB
	bc.OnlyA = blackPoints(psa, onlyA, evA)
	bc.OnlyB = blackPoints(psb, onlyB, evB)

	internalA := hasInternal(onlyA, len(psa))
	internalB := hasInternal(onlyB, len(psb))
	onlyABlack, onlyADef, _ := runBlackness(onlyA, evA)
	onlyBBlack, onlyBDef, _ := runBlackness(onlyB, evB)
	switch {
	case onlyADef && len(onlyA) > 0 && len(onlyB) == 0 && onlyABlack:
		bc.Classification = "extra-frames-are-black"
		bc.Explanation = "多出的视频帧全部由逐帧 sidecar 证明为黑帧，属于 lead/tail handle 差异，不是画面内容漏帧"
	case len(onlyA) > 0 && onlyABlack && evA.Summary.Source == "sample-size-candidate":
		bc.Classification = "black-candidates"
		bc.Explanation = "多出的视频帧体积异常小；这是黑帧候选，不是像素级结论"
	case onlyBDef && len(onlyB) > 0 && len(onlyA) == 0 && onlyBBlack:
		bc.Classification = "extra-frames-are-black"
		bc.Explanation = "多出的视频帧全部由逐帧 sidecar 证明为黑帧，属于 lead/tail handle 差异，不是画面内容漏帧"
	case len(onlyA)+len(onlyB) == 0:
		bc.Classification = "no-extra-frames"
		bc.Explanation = "两侧 presented 视频包序列一致，没有多帧/缺包"
	default:
		bc.Classification = "not-proven-black"
		bc.Explanation = "多出的视频帧未被证明是黑帧，不能按合法封装差异忽略"
	}

	switch {
	case !out.FPSMatch && (len(onlyA) > 0 || len(onlyB) > 0):
		out.Classification = "frame-rate-mismatch"
		out.Explanation = fmt.Sprintf("帧率不同（A=%s, B=%s）；先修正帧率，再判断尾部帧数", out.FPSA, out.FPSB)
	case !al.Comparable:
		out.Classification = "timing-only-unverified"
		out.Explanation = "编码包内容无法可靠对齐，只能比较时间线；不能据此宣称漏帧"
	case internalA || internalB:
		out.Classification = "real-frame-loss-or-insert"
		out.Explanation = "差异位于视频序列内部，不是片头/片尾 edit 或黑场 handle；属于真实漏帧/插帧"
	case len(onlyA) == 0 && len(onlyB) == 0:
		if math.Abs(out.Timeline.Delta) > opts.ToleranceMS {
			out.Classification = "timeline-shift-only"
			out.Explanation = fmt.Sprintf("视频包完全相同，仅 presented 起点相差 %.3f ms", out.Timeline.Delta)
		} else {
			out.Classification = "identical-payload-and-timeline"
			out.Explanation = "视频包序列和 presented 起点一致"
		}
	case onlyADef && len(onlyA) > 0 && len(onlyB) == 0 && onlyABlack:
		out.Classification = "black-handle-only"
		out.Explanation = fmt.Sprintf("%s 多出 %d 帧黑场，属于容器/剪辑尾部 handle", opts.LabelA, len(onlyA))
	case onlyBDef && len(onlyB) > 0 && len(onlyA) == 0 && onlyBBlack:
		out.Classification = "black-handle-only"
		out.Explanation = fmt.Sprintf("%s 多出 %d 帧黑场，属于容器/剪辑尾部 handle", opts.LabelB, len(onlyB))
	case len(onlyA) > 0 && len(onlyB) == 0 && onlyABlack:
		out.Classification = "black-handle-candidate"
		out.Explanation = fmt.Sprintf("%s 多出 %d 个黑帧候选，需用逐帧 sidecar 确认", opts.LabelA, len(onlyA))
	case len(onlyB) > 0 && len(onlyA) == 0 && onlyBBlack:
		out.Classification = "black-handle-candidate"
		out.Explanation = fmt.Sprintf("%s 多出 %d 个黑帧候选，需用逐帧 sidecar 确认", opts.LabelB, len(onlyB))
	default:
		out.Classification = edgeClassification(onlyA, onlyB, len(psa), len(psb))
		out.Explanation = edgeExplanation(onlyA, onlyB, opts)
		if likelyMissingTail(va, vb, aa, ab, onlyA, onlyB) {
			out.Classification = "likely-missing-tail"
			out.Explanation = "多出的帧位于片尾，且短版视频终点明显早于同文件音频/字幕终点；更像短版尾部真实漏帧"
		}
	}
	return out, bc
}

func blackPoints(seq []media.PresentedSample, runs []FrameRun, ev frameEvidence) []BlackFramePoint {
	var out []BlackFramePoint
	for _, run := range runs {
		for i := run.StartIndex; i < run.EndIndex && i < len(seq); i++ {
			p := BlackFramePoint{
				AtMs: ms(seq[i].Start), EndMs: ms(seq[i].End),
				Hash: fmt.Sprintf("%016x", seq[i].Hash),
			}
			if i < len(ev.flags) {
				p.Black = ev.flags[i]
				p.Definitive = ev.flags[i] && ev.Summary.Definitive
			}
			out = append(out, p)
		}
	}
	return out
}

func hasInternal(runs []FrameRun, total int) bool {
	for _, r := range runs {
		if r.StartIndex > 0 && r.EndIndex < total {
			return true
		}
	}
	return false
}

func runBlackness(runs []FrameRun, ev frameEvidence) (all, definitive, any bool) {
	if len(runs) == 0 {
		return false, false, false
	}
	all = true
	definitive = ev.Summary.Definitive
	for _, r := range runs {
		for i := r.StartIndex; i < r.EndIndex; i++ {
			if i < 0 || i >= len(ev.flags) || !ev.flags[i] {
				all = false
				continue
			}
			any = true
		}
	}
	if !all {
		definitive = false
	}
	return all, definitive, any
}

func edgeClassification(a, b []FrameRun, na, nb int) string {
	head, tail := false, false
	for _, r := range a {
		head = head || r.StartIndex == 0
		tail = tail || r.EndIndex == na
	}
	for _, r := range b {
		head = head || r.StartIndex == 0
		tail = tail || r.EndIndex == nb
	}
	switch {
	case head && tail:
		return "head-and-tail-frame-difference"
	case head:
		return "head-frame-difference"
	case tail:
		return "tail-frame-difference"
	default:
		return "edge-frame-difference"
	}
}

func edgeExplanation(a, b []FrameRun, opts Options) string {
	var parts []string
	if len(a) > 0 {
		parts = append(parts, fmt.Sprintf("%s 独有 %d 帧", opts.LabelA, len(a)))
	}
	if len(b) > 0 {
		parts = append(parts, fmt.Sprintf("%s 独有 %d 帧", opts.LabelB, len(b)))
	}
	return strings.Join(parts, "；") + "，差异在片头/片尾但不是黑场，不能仅凭封装判定合法"
}

func likelyMissingTail(va, vb, aa, ab *media.Track, onlyA, onlyB []FrameRun) bool {
	if len(onlyA) == 0 && len(onlyB) == 0 {
		return false
	}
	pa, pb := va.PresentedSpan(), vb.PresentedSpan()
	frameA, frameB := va.FrameDurationNanos(), vb.FrameDurationNanos()
	if aa != nil && len(onlyA) > 0 {
		audioEnd := aa.PresentedSpan().End
		if absInt64(pa.End-audioEnd) <= maxInt64(frameA*3/2, 1) &&
			pb.End+int64(extraDuration(onlyA)*1e6)+frameB < abEnd(ab) {
			return true
		}
	}
	if ab != nil && len(onlyB) > 0 {
		audioEnd := ab.PresentedSpan().End
		if absInt64(pb.End-audioEnd) <= maxInt64(frameB*3/2, 1) &&
			pa.End+int64(extraDuration(onlyB)*1e6)+frameA < abEnd(aa) {
			return true
		}
	}
	return false
}

func abEnd(t *media.Track) int64 {
	if t == nil {
		return 0
	}
	return t.PresentedSpan().End
}

func absInt64(v int64) int64 {
	if v < 0 {
		return -v
	}
	return v
}

func maxInt64(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}

func compareAudio(aa, ab *media.Track, opts Options) AudioComparison {
	out := AudioComparison{Timeline: timelinePair(aa, ab, false)}
	if aa == nil || ab == nil {
		out.Classification = "missing-audio-track"
		out.Explanation = "至少一侧没有音频轨"
		return out
	}
	pa, pb := aa.PresentedSpan(), ab.PresentedSpan()
	out.PacketsA, out.PacketsB = len(aa.Samples), len(ab.Samples)
	out.DurationDeltaMs = ms((pb.End - pb.Start) - (pa.End - pa.Start))
	out.SampleFramesA = decodedAudioSamples(pa.End-pa.Start, aa.SampleRate)
	out.SampleFramesB = decodedAudioSamples(pb.End-pb.Start, ab.SampleRate)
	out.SampleFrameDelta = out.SampleFramesB - out.SampleFramesA
	out.HeadTrimA, out.HeadTrimB = ms(aa.HeadTrim()), ms(ab.HeadTrim())
	out.TailTrimA, out.TailTrimB = ms(aa.TailTrim()), ms(ab.TailTrim())
	out.CodecDelayA, out.CodecDelayB = ms(aa.CodecDelay), ms(ab.CodecDelay)
	al, onlyA, onlyB := alignTracks(aa, ab)
	out.Identity, out.OnlyA, out.OnlyB = al, onlyA, onlyB
	internal := hasInternal(onlyA, len(aa.PresentedSamples())) ||
		hasInternal(onlyB, len(ab.PresentedSamples()))
	switch {
	case !al.Comparable:
		if math.Abs(out.DurationDeltaMs) > opts.ToleranceMS {
			out.Classification = "timing-difference-unverified"
			out.Explanation = fmt.Sprintf(
				"音频 presented 时长相差 %+.3f ms，但编码包不能可靠对齐；先核对 edit list/CodecDelay，不能直接判定漏样本",
				out.DurationDeltaMs)
		} else {
			out.Classification = "timeline-match-unverified"
			out.Explanation = "音频时间线一致，但编码包内容无法可靠对齐"
		}
	case internal:
		out.Classification = "real-audio-loss-or-insert"
		out.Explanation = "音频包序列内部存在缺失/插入，不是片头 priming 或片尾 padding 边界"
	case len(onlyA) == 0 && len(onlyB) == 0:
		if math.Abs(out.DurationDeltaMs) > opts.ToleranceMS {
			out.Classification = "edit-or-discard-padding-only"
			out.Explanation = fmt.Sprintf(
				"音频编码包完全一致，仅 presented 时长相差 %+.3f ms；差异来自 edit list、CodecDelay 或 DiscardPadding，不是漏音频样本",
				out.DurationDeltaMs)
		} else if math.Abs(out.Timeline.Delta) > opts.ToleranceMS {
			out.Classification = "codec-delay-offset-only"
			out.Explanation = fmt.Sprintf(
				"音频包一致，起点相差 %+.3f ms；属于编码 delay/priming 补偿口径差异",
				out.Timeline.Delta)
		} else {
			out.Classification = "identical-payload-and-timeline"
			out.Explanation = "音频包和 presented 时间线一致"
		}
	default:
		headA, tailA := runEdges(onlyA, len(aa.PresentedSamples()))
		headB, tailB := runEdges(onlyB, len(ab.PresentedSamples()))
		extra := extraDuration(onlyA) + extraDuration(onlyB)
		switch {
		case (headA || headB) && extra <= 120:
			out.Classification = "priming-boundary"
			out.Explanation = fmt.Sprintf(
				"音频只在片头多出 %.3f ms 包，符合 encoder priming/CodecDelay 边界，不是内容漏样本", extra)
		case (tailA || tailB) && extra <= 120:
			out.Classification = "padding-boundary"
			out.Explanation = fmt.Sprintf(
				"音频只在片尾多出 %.3f ms 包，符合 encoder padding/DiscardPadding/edit 尾部边界", extra)
		default:
			out.Classification = "audio-edge-difference"
			out.Explanation = fmt.Sprintf(
				"音频包差异位于片头/片尾，长度 %.3f ms；需核对目标交付是否应裁掉", extra)
		}
	}
	return out
}

func decodedAudioSamples(durationNS int64, sampleRate int) int64 {
	if durationNS <= 0 || sampleRate <= 0 {
		return 0
	}
	return int64(math.Round(float64(durationNS) * float64(sampleRate) / 1e9))
}

func runEdges(runs []FrameRun, total int) (head, tail bool) {
	for _, r := range runs {
		head = head || r.StartIndex == 0
		tail = tail || r.EndIndex == total
	}
	return head, tail
}

func extraDuration(runs []FrameRun) float64 {
	var total float64
	for _, r := range runs {
		if r.EndMs >= r.StartMs {
			total += r.EndMs - r.StartMs
		}
	}
	return total
}

func compareSubtitle(sa, sb *media.Track, opts Options) SubtitleComparison {
	out := SubtitleComparison{Timeline: timelinePair(sa, sb, true)}
	out.PresentA, out.PresentB = sa != nil && out.Timeline.A.Samples > 0, sb != nil && out.Timeline.B.Samples > 0
	switch {
	case !out.PresentA && !out.PresentB:
		out.Classification = "both-absent"
		out.Explanation = "两侧都没有字幕轨"
	case out.PresentA != out.PresentB:
		out.Classification = "subtitle-track-presence-difference"
		out.Explanation = "字幕轨只出现在一侧"
	case math.Abs(out.Timeline.Delta) <= math.Max(opts.ToleranceMS, 10):
		out.Classification = "in-point-match"
		out.Explanation = fmt.Sprintf("字幕首个非空样本入点相差 %+.3f ms，在容差内", out.Timeline.Delta)
	default:
		out.Classification = "in-point-shift"
		out.Explanation = fmt.Sprintf("字幕入点相差 %+.3f ms，应沿 edit list/timestamp offset 查", out.Timeline.Delta)
	}
	return out
}

func buildFindings(rep *Report, opts Options) []Finding {
	var out []Finding
	add := func(sev, code, msg string) {
		out = append(out, Finding{Severity: sev, Code: code, Message: msg})
	}
	if rep.Video.Classification != "" {
		add(videoSeverity(rep.Video.Classification),
			"VIDEO_"+strings.ToUpper(strings.ReplaceAll(rep.Video.Classification, "-", "_")),
			rep.Video.Explanation)
	}
	if !rep.Video.FPSMatch && rep.Video.FPSA != "" {
		add("warn", "FRAME_RATE_MISMATCH",
			fmt.Sprintf("视频帧率 A=%s (%0.6f), B=%s (%0.6f)；帧率差会随时间累积成多帧",
				rep.Video.FPSA, fpsFloat(rep.Video.FPSA),
				rep.Video.FPSB, fpsFloat(rep.Video.FPSB)))
	}
	if rep.Audio.Classification != "" {
		add(audioSeverity(rep.Audio.Classification),
			"AUDIO_"+strings.ToUpper(strings.ReplaceAll(rep.Audio.Classification, "-", "_")),
			rep.Audio.Explanation)
	}
	if math.Abs(rep.Audio.HeadTrimA-rep.Audio.HeadTrimB) > opts.ToleranceMS {
		add("info", "AUDIO_HEAD_TRIM_MISMATCH",
			fmt.Sprintf("音频头部裁剪 A=%.3f ms, B=%.3f ms；优先核对 MP4 elst.media_time 与 MKV CodecDelay",
				rep.Audio.HeadTrimA, rep.Audio.HeadTrimB))
	}
	if math.Abs(rep.Audio.TailTrimA-rep.Audio.TailTrimB) > opts.ToleranceMS {
		add("info", "AUDIO_TAIL_TRIM_MISMATCH",
			fmt.Sprintf("音频尾部裁剪 A=%.3f ms, B=%.3f ms；优先核对 MP4 edit segment_duration 与 MKV DiscardPadding",
				rep.Audio.TailTrimA, rep.Audio.TailTrimB))
	}
	if rep.Subtitle.Classification != "" && rep.Subtitle.Classification != "both-absent" {
		add(severityForSubtitle(rep.Subtitle.Classification),
			"SUBTITLE_"+strings.ToUpper(strings.ReplaceAll(rep.Subtitle.Classification, "-", "_")),
			rep.Subtitle.Explanation)
	}
	if math.Abs(rep.TotalDuration.Delta) > opts.ToleranceMS {
		add("warn", "TOTAL_DURATION_DELTA",
			fmt.Sprintf("按 presented 轨计算的总终点 A=%.3f ms, B=%.3f ms（A-B=%+.3f ms）",
				rep.TotalDuration.A.MeasuredMs, rep.TotalDuration.B.MeasuredMs, rep.TotalDuration.Delta))
	}
	for _, d := range []struct {
		label string
		v     DurationBreakdown
	}{{"A", rep.TotalDuration.A}, {"B", rep.TotalDuration.B}} {
		if diff := d.v.MeasuredMs - d.v.DeclaredMs; math.Abs(diff) > math.Max(opts.ToleranceMS, 1) {
			add("info", "CONTAINER_DURATION_"+d.label,
				fmt.Sprintf("%s 容器声明时长 %.3f ms，presented 实测终点 %.3f ms（差 %+.3f ms）",
					d.label, d.v.DeclaredMs, d.v.MeasuredMs, diff))
		}
	}
	for _, side := range []struct {
		code string
		ev   BlackEvidence
	}{{"A", rep.BlackFrames.A}, {"B", rep.BlackFrames.B}} {
		if side.ev.Warning != "" {
			add("info", "BLACK_EVIDENCE_"+side.code, side.ev.Warning)
		}
	}
	if rep.ContainerA.Format != rep.ContainerB.Format {
		add("info", "CONTAINER_DIFFERENCE",
			fmt.Sprintf("容器不同：A=%s, B=%s；时间尺度、edit list/CodecDelay 表达方式不同，必须按 presented 时间比较",
				rep.ContainerA.Format, rep.ContainerB.Format))
	}
	return out
}

func fpsFloat(s string) float64 {
	if i := strings.IndexByte(s, '/'); i >= 0 {
		var n, d float64
		fmt.Sscanf(s, "%f/%f", &n, &d)
		if d != 0 {
			return n / d
		}
	}
	var v float64
	fmt.Sscanf(s, "%f", &v)
	return v
}

func videoSeverity(class string) string {
	switch class {
	case "real-frame-loss-or-insert", "likely-missing-tail":
		return "error"
	case "edge-frame-difference", "head-frame-difference", "tail-frame-difference",
		"head-and-tail-frame-difference", "timing-only-unverified",
		"frame-rate-mismatch":
		return "warn"
	case "black-handle-candidate":
		return "warn"
	default:
		return "info"
	}
}

func audioSeverity(class string) string {
	switch class {
	case "real-audio-loss-or-insert":
		return "error"
	case "timing-difference-unverified", "audio-edge-difference":
		return "warn"
	default:
		return "info"
	}
}

func severityForSubtitle(class string) string {
	switch class {
	case "subtitle-track-presence-difference", "in-point-shift":
		return "warn"
	default:
		return "info"
	}
}

func decideVerdict(rep *Report) Verdict {
	v := Verdict{}
	realVideo := rep.Video.Classification == "real-frame-loss-or-insert" ||
		rep.Video.Classification == "likely-missing-tail"
	realAudio := rep.Audio.Classification == "real-audio-loss-or-insert"
	v.RealLoss = realVideo || realAudio
	for _, f := range rep.Findings {
		switch f.Severity {
		case "warn":
			v.NeedsReview = true
		}
	}
	switch {
	case v.RealLoss:
		v.Status = "real-difference"
		v.Summary = fmt.Sprintf("检测到真实媒体序列差异：视频 %s，音频 %s",
			rep.Video.Classification, rep.Audio.Classification)
	case v.NeedsReview:
		v.Status = "needs-review"
		v.Summary = fmt.Sprintf("尚未证明漏帧，但存在需人工确认的边界：视频 %s，音频 %s",
			rep.Video.Classification, rep.Audio.Classification)
	default:
		v.Status = "legal-mux-difference"
		v.LegalMux = true
		v.Summary = "差异可由容器时间线、编码 delay、edit list/DiscardPadding 或黑场 handle 解释，未发现序列内部漏包"
	}
	return v
}

func nextCheck(rep *Report) string {
	var steps []string
	if rep.Video.Classification == "frame-rate-mismatch" {
		steps = append(steps, fmt.Sprintf("先统一帧率：A=%s，B=%s", rep.Video.FPSA, rep.Video.FPSB))
	}
	if len(rep.Video.OnlyA) > 0 {
		r := rep.Video.OnlyA[0]
		steps = append(steps, fmt.Sprintf("检查 A 视频 %.3f–%.3f ms 的 %d 帧是否为黑场；若不是，B 在该处缺失真实画面",
			r.StartMs, r.EndMs, r.Frames))
	}
	if len(rep.Video.OnlyB) > 0 {
		r := rep.Video.OnlyB[0]
		steps = append(steps, fmt.Sprintf("检查 B 视频 %.3f–%.3f ms 的 %d 帧是否为黑场；若不是，A 在该处缺失真实画面",
			r.StartMs, r.EndMs, r.Frames))
	}
	if math.Abs(rep.Audio.DurationDeltaMs) > 1 || math.Abs(rep.Audio.Timeline.Delta) > 1 {
		steps = append(steps, fmt.Sprintf("核对音频：presented 时长 A-B=%+.3f ms，起点 A-B=%+.3f ms；查 MP4 elst 与 MKV CodecDelay/DiscardPadding",
			-rep.Audio.DurationDeltaMs, rep.Audio.Timeline.Delta))
	}
	if rep.Subtitle.Classification == "in-point-shift" {
		steps = append(steps, fmt.Sprintf("核对字幕入点 A=%.3f ms，B=%.3f ms", rep.Subtitle.Timeline.A.StartMs, rep.Subtitle.Timeline.B.StartMs))
	}
	if len(steps) == 0 {
		steps = append(steps, "复核两侧 start_time、edit list/CodecDelay 与末帧 duration；当前未发现序列内部漏包")
	}
	if (len(rep.Video.OnlyA) > 0 || len(rep.Video.OnlyB) > 0) &&
		(!rep.BlackFrames.A.Definitive || !rep.BlackFrames.B.Definitive) {
		steps = append(steps, "补充 -frames-a/-frames-b 逐帧 CSV 后再判断多出的帧是否为黑场")
	}
	return strings.Join(steps, "；")
}
