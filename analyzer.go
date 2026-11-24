package mp4diag

import (
	"fmt"
	"os"
)

type Options struct {
	Path      string
	Reference string
}

func Analyze(path string) (*Report, error) {
	return AnalyzeOptions(Options{Path: path})
}

func AnalyzeOptions(options Options) (*Report, error) {
	info, err := os.Stat(options.Path)
	if err != nil {
		return nil, err
	}
	if info.IsDir() {
		return nil, fmt.Errorf("%s is a directory", options.Path)
	}
	file, err := os.Open(options.Path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var boxes []BoxInfo
	var findings []Finding
	scanBoxes(file, 0, info.Size(), "", 0, &boxes, &findings)
	samples := buildSamples(file, info.Size(), boxes, &findings)

	report := &Report{
		SchemaVersion: SchemaVersion,
		Path:          options.Path,
		FileSize:      info.Size(),
		BoxCount:      len(boxes),
		Boxes:         boxes,
		Samples:       samples,
		Findings:      findings,
		Layers:        map[string]LayerSummary{},
		Display: DisplayImpact{
			Method:     "stss_sync_sample_model",
			Confidence: "structural_only",
			Note:       "未提供参考文件或解码器时，只给 GOP/同步样本层面的静态范围，不作出像素损坏结论",
		},
	}

	for i := range report.Boxes {
		box := &report.Boxes[i]
		if !box.Valid && report.FirstBadBox == nil {
			report.FirstBadBox = &BoxLocator{Path: box.Path, Type: box.Type, Offset: box.Offset}
		}
	}

	if options.Reference != "" {
		refReport, err := Analyze(options.Reference)
		if err != nil {
			return nil, fmt.Errorf("analyze reference: %w", err)
		}
		reference := compareReference(file, info.Size(), report, refReport)
		report.Reference = &reference
		appendReferenceFindings(report, refReport)
		report.Display = displayWithReference(report, refReport)
	} else {
		report.Display = displayWithoutReference(report)
	}

	appendDisplayFindings(report)
	finalizeFindingsAndLayers(report)
	return report, nil
}

func finalizeFindingsAndLayers(report *Report) {
	for i := range report.Findings {
		report.Findings[i].ID = fmt.Sprintf("F%03d", i+1)
	}

	idsByLayer := map[string][]string{}
	statusByLayer := map[string]string{}
	summaryByLayer := map[string]string{}
	firstBoxByLayer := map[string]*BoxLocator{}

	for _, finding := range report.Findings {
		idsByLayer[finding.Layer] = append(idsByLayer[finding.Layer], finding.ID)
		current := statusByLayer[finding.Layer]
		if finding.Severity == SeverityError {
			statusByLayer[finding.Layer] = StatusError
		} else if finding.Severity == SeverityWarning && current != StatusError {
			statusByLayer[finding.Layer] = StatusWarning
		} else if current == "" {
			statusByLayer[finding.Layer] = StatusOK
		}
		if finding.Box != nil && firstBoxByLayer[finding.Layer] == nil {
			firstBoxByLayer[finding.Layer] = finding.Box
		}
	}

	ensure := func(layer, okSummary string) {
		if statusByLayer[layer] == "" {
			statusByLayer[layer] = StatusOK
			summaryByLayer[layer] = okSummary
		}
	}
	ensure(LayerContainer, "box 边界与嵌套扫描未发现结构错误")
	ensure(LayerSampleTable, "样本表计数、chunk 映射和文件边界检查通过")
	if metadataFinding := findMetadataReferenceFinding(report); metadataFinding != nil && metadataFinding.Box != nil {
		firstBoxByLayer[LayerSampleTable] = metadataFinding.Box
	}
	if statusByLayer[LayerCompressed] == "" {
		statusByLayer[LayerCompressed] = StatusNotAssessed
		summaryByLayer[LayerCompressed] = "缺少已知良好参考或码流解码器；未把容器/样本表异常推断为像素损坏"
	}
	if report.Reference != nil {
		switch report.Reference.PixelDamage {
		case PixelRuledOut:
			statusByLayer[LayerCompressed] = StatusOK
			summaryByLayer[LayerCompressed] = "参考 sample 区间 SHA-256 全部一致；元数据异常已与压缩/像素层分离"
		case PixelSuspected:
			statusByLayer[LayerCompressed] = StatusError
			summaryByLayer[LayerCompressed] = "至少一个 sample 的压缩字节与参考不同；需解码器/帧哈希确认最终花屏"
		case PixelNotAssessed:
			statusByLayer[LayerCompressed] = StatusNotAssessed
			summaryByLayer[LayerCompressed] = "部分 sample 缺失，其余可用区间已做哈希；缺失不等于像素翻转"
		}
	}
	if statusByLayer[LayerDisplay] == "" {
		statusByLayer[LayerDisplay] = StatusOK
		summaryByLayer[LayerDisplay] = "没有已知压缩样本故障；显示顺序和同步样本模型可用"
	}

	for _, layer := range []string{LayerContainer, LayerSampleTable, LayerCompressed, LayerDisplay} {
		summary := summaryByLayer[layer]
		if summary == "" {
			summary = defaultLayerSummary(layer, statusByLayer[layer], report)
		}
		report.Layers[layer] = LayerSummary{
			Status:      statusByLayer[layer],
			Summary:     summary,
			FindingIDs:  idsByLayer[layer],
			FirstBadBox: firstBoxByLayer[layer],
		}
	}
}

func defaultLayerSummary(layer, status string, report *Report) string {
	switch layer {
	case LayerContainer:
		if status == StatusError {
			return "发现首个越界/非法 box；目录 box 可能仍可读取"
		}
	case LayerSampleTable:
		if status == StatusError || status == StatusWarning {
			return "样本表存在不一致；这是元数据故障，不直接等于像素损坏"
		}
	case LayerDisplay:
		if len(report.Display.AffectedDisplayFrames) > 0 {
			return "按同步样本/参考依赖模型得到受影响显示帧和恢复点"
		}
		return "没有预测到像素传播区间；元数据修复后应按参考帧哈希复核"
	case LayerCompressed:
		return "压缩数据层已有结论；详见 findings 与 reference 哈希"
	}
	return ""
}

func displayWithoutReference(report *Report) DisplayImpact {
	var missing []int
	hasTableError := false
	containerTruncation := false
	for _, finding := range report.Findings {
		if finding.Code == "samples_past_truncated_box" {
			containerTruncation = true
			missing = append(missing, finding.Samples...)
		}
		if finding.Layer == LayerSampleTable && finding.Severity == SeverityError {
			hasTableError = true
		}
	}

	impact := DisplayImpact{Method: "stss_sync_sample_model", Confidence: "structural_only"}
	if containerTruncation {
		impact.FaultSamples = missing
		impact.AffectedSamples = missing
		impact.AffectedDisplayFrames = missing
		impact.CleanSampleRanges = complementSpans(len(report.Samples), missing)
		impact.CleanDisplayFrameRanges = impact.CleanSampleRanges
		impact.Confidence = "high_for_available_prefix"
		impact.Note = "容器截断后的 sample 不可读取；这些是数据缺失，不是已证实的像素翻转"
		return impact
	}
	if hasTableError {
		impact.Confidence = "not_assessed"
		impact.Note = "样本表映射不可信；需参考文件或修复元数据后才能判断像素和显示范围"
		return impact
	}
	impact.CleanSampleRanges = []Span{{First: 1, Last: len(report.Samples)}}
	impact.CleanDisplayFrameRanges = impact.CleanSampleRanges
	impact.Note = "未发现表或容器错误；仍需参考哈希/解码器才能排除压缩数据损坏"
	return impact
}

func complementSpans(total int, removed []int) []Span {
	if total == 0 {
		return nil
	}
	set := map[int]bool{}
	for _, n := range removed {
		set[n] = true
	}
	var clean []int
	for n := 1; n <= total; n++ {
		if !set[n] {
			clean = append(clean, n)
		}
	}
	return consecutiveSpans(clean)
}
