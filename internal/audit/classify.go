package audit

import (
	"fmt"
	"strings"
)

var (
	limitedRange = LegalRange{Name: "limited", Black: 16, White: 235}
	fullRange    = LegalRange{Name: "full", Black: 0, White: 255}
)

func ParseDeclared(value string) (DeclaredRange, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", "unknown", "auto", "unset":
		return DeclaredUnknown, nil
	case "full", "pc", "jpeg", "0-255":
		return DeclaredFull, nil
	case "limited", "video", "tv", "16-235":
		return DeclaredLimited, nil
	default:
		return DeclaredUnknown, fmt.Errorf("invalid declared range %q (want full, limited, or unknown)", value)
	}
}

func Assess(declared DeclaredRange, s SampleStats) Assessment {
	if declared == "" {
		declared = DeclaredUnknown
	}
	limitedScore, limitedEvidence := scoreLimited(s)
	fullScore, fullEvidence := scoreFull(s)
	limited := blackLevel(s, limitedRange)
	full := blackLevel(s, fullRange)
	limitedHyp := Hypothesis{
		Key:            "limited_16_235",
		Range:          limitedRange,
		Score:          limitedScore,
		Summary:        "样本是 limited/video-range；若播放器声明 full，则该声明属于标签错误",
		CorrectedLabel: "limited (16-235)",
		BlackLevel:     limited,
		Evidence:       limitedEvidence,
	}
	fullHyp := Hypothesis{
		Key:            "full_0_255_lifted_or_stylized",
		Range:          fullRange,
		Score:          fullScore,
		Summary:        "样本是真正的 full-range；低端被抬黑，或创作时刻意避开纯黑",
		CorrectedLabel: "full (0-255)",
		BlackLevel:     full,
		Evidence:       fullEvidence,
	}

	decision, confidence := decide(limitedScore, fullScore)
	a := Assessment{
		Declared:    declared,
		Decision:    decision,
		Confidence:  confidence,
		Hypotheses:  []Hypothesis{limitedHyp, fullHyp},
		Evidence:    rangeEvidence(s),
		Caveats:     defaultCaveats(),
		LabelStatus: labelStatus(declared, decision, confidence),
	}
	switch decision {
	case "limited":
		a.CorrectedLabel = "limited (16-235)"
		a.Summary = "证据倾向 limited 编码范围。除非外部元数据能证明这是特殊 full-range 编码，否则播放器的 full 标签应改为 limited。"
	case "full":
		a.CorrectedLabel = "full (0-255)"
		a.Summary = "越界样本和端点证据倾向 full 信号；暗部抬高应解释为内容、真正抬黑或风格化处理，而不是范围标签错误。"
	default:
		a.CorrectedLabel = "undetermined: limited (16-235) or full (0-255)"
		a.Summary = "仅凭直方图无法区分“limited 样片被错标成 full”与“true full-range 样片抬黑/风格化暗部”。两种解释均保留。"
	}
	if confidence == "medium" {
		a.Summary += " 由于证据优势有限，此结论是暂定的。"
	}
	return a
}

func decide(limitedScore, fullScore int) (string, string) {
	diff := limitedScore - fullScore
	switch {
	case diff >= 4 && limitedScore >= 8:
		return "limited", "high"
	case diff >= 3 && limitedScore >= 6:
		return "limited", "medium"
	case -diff >= 4 && fullScore >= 8:
		return "full", "high"
	case -diff >= 3 && fullScore >= 6:
		return "full", "medium"
	default:
		return "ambiguous", "low"
	}
}

func labelStatus(declared DeclaredRange, decision, confidence string) string {
	if decision == "ambiguous" {
		return "unverified"
	}
	if declared == DeclaredUnknown {
		if confidence == "high" {
			return "inferred"
		}
		return "provisional"
	}
	matches := (declared == DeclaredFull && decision == "full") ||
		(declared == DeclaredLimited && decision == "limited")
	if matches {
		if confidence == "high" {
			return "correct"
		}
		return "likely-correct"
	}
	if confidence == "high" {
		return "incorrect"
	}
	return "likely-incorrect"
}
