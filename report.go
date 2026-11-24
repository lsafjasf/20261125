package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type FrameArtifact struct {
	Role        string `json:"role"`
	File        string `json:"file"`
	Description string `json:"description"`
	Width       int    `json:"width"`
	Height      int    `json:"height"`
	SHA256      string `json:"sha256"`
}

type Report struct {
	SchemaVersion  string          `json:"schemaVersion"`
	Tool           string          `json:"tool"`
	GoVersion      string          `json:"goVersion"`
	CreatedUTC     string          `json:"createdUtc"`
	OutputDir      string          `json:"outputDirectory"`
	FFmpegVersion  string          `json:"ffmpegVersion,omitempty"`
	FFProbeVersion string          `json:"ffprobeVersion,omitempty"`
	Warnings       []string        `json:"warnings,omitempty"`
	Observation    Observation     `json:"observation"`
	Combinations   []ComboCheck    `json:"doubleRotationCombinations"`
	HighRisk       []ComboCheck    `json:"highRiskCombinations"`
	Frames         []FrameArtifact `json:"frames"`
	Conclusion     string          `json:"conclusion"`
	NextSteps      []string        `json:"nextSteps"`
	RepairCommands []string        `json:"repairCommands"`
}

func frameArtifact(role, path, description string) (FrameArtifact, error) {
	width, height, _ := readImageConfig(path)
	hash, err := hashFile(path)
	if err != nil {
		return FrameArtifact{}, err
	}
	return FrameArtifact{
		Role: role, File: filepath.Base(path), Description: description,
		Width: width, Height: height, SHA256: hash,
	}, nil
}

func hashFile(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}

func writeReportFiles(outDir string, report *Report) error {
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return err
	}
	jsonPath := filepath.Join(outDir, "rotation-report.json")
	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(jsonPath, append(data, '\n'), 0o644); err != nil {
		return err
	}
	mdPath := filepath.Join(outDir, "rotation-report.md")
	return os.WriteFile(mdPath, []byte(renderMarkdown(report)), 0o644)
}

func renderMarkdown(r *Report) string {
	var b strings.Builder
	o := r.Observation
	fmt.Fprintf(&b, "# 旋转/显示矩阵对照报告\n\n")
	fmt.Fprintf(&b, "- 输入：`%s`\n", o.InputPath)
	fmt.Fprintf(&b, "- 结论：%s\n\n", r.Conclusion)

	b.WriteString("## 三个必须分开的事实\n\n")
	b.WriteString("| 检查项 | 值 |\n|---|---|\n")
	fmt.Fprintf(&b, "| 真实像素网格（`-noautorotate` 抽帧） | `%dx%d` / %s |\n", o.PixelWidth, o.PixelHeight, o.PixelOrient)
	fmt.Fprintf(&b, "| FFprobe 编码宽高 | `%dx%d` |\n", o.CodedWidth, o.CodedHeight)
	if o.HasTrackDisplay {
		fmt.Fprintf(&b, "| `tkhd` 显示宽高 | `%.2fx%.2f` / %s |\n", o.TrackDisplayWidth, o.TrackDisplayHeight, o.TrackDisplayOrient)
		if o.TrackImpliedSAR > 0 {
			fmt.Fprintf(&b, "| 由 `tkhd`/编码宽高反推 SAR | `%.4f`（极端非 1 表示显示宽高与像素网格被额外拉伸缩放） |\n", o.TrackImpliedSAR)
		}
	} else {
		b.WriteString("| `tkhd` 显示宽高 | 未找到（非 MP4/MOV 或盒子缺失） |\n")
	}
	fmt.Fprintf(&b, "| 显示宽高声明来源 | `%s` / %s |\n", o.DisplayClaimSource, o.DisplayClaimOrient)
	if o.MP4Matrix != nil {
		fmt.Fprintf(&b, "| MP4 旋转矩阵原始值 | `%v` |\n", o.MP4Matrix.Raw)
	}
	if o.FFProbeMatrix != nil && o.FFProbeMatrix.FFprobeRotation != nil {
		fmt.Fprintf(&b, "| FFprobe rotation | `%.1f°`（本工具换算为顺时针 %d/4 圈） |\n", *o.FFProbeMatrix.FFprobeRotation, o.MatrixTurnsCW)
	}
	fmt.Fprintf(&b, "| 生效矩阵旋转 | 顺时针 `%d°` / %d quarter-turn(s)，换轴=%v |\n", o.MatrixDegCW, o.MatrixTurnsCW, o.MatrixSwapsAxes)
	fmt.Fprintf(&b, "| 矩阵后几何尺寸 | `%dx%d` / %s |\n", o.MatrixOutputWidth, o.MatrixOutputHeight, o.MatrixOutputOrient)
	fmt.Fprintf(&b, "| SAR / DAR | `%s` / `%s` |\n", valueOrDash(o.SAR), valueOrDash(o.DAR))
	if o.Pixel.CorrectionTurnsCW != nil {
		fmt.Fprintf(&b, "| 真实像素到正向还需 | 顺时针 `%d°`（来源：`%s`） |\n", *o.Pixel.CorrectionDegCW, o.Pixel.Source)
	} else {
		b.WriteString("| 真实像素语义方向 | 未知；需 `-reference` 正向参考图或 `-pixel-correction-cw` |\n")
	}
	if o.ResidualTurnsCW != nil {
		fmt.Fprintf(&b, "| 播放器遵循矩阵后残余旋转 | `%d` quarter-turn(s) |\n", *o.ResidualTurnsCW)
	}
	b.WriteString("\n")

	b.WriteString("## 二次旋转组合判定\n\n")
	b.WriteString("| ID | 范围 | 组合 | 状态 | 严重度 | 分析/修复 |\n|---|---|---|---|---|---|\n")
	for _, c := range r.Combinations {
		fmt.Fprintf(&b, "| `%s` | `%s` | %s | `%s` | `%s` | %s **修复：** %s |\n",
			c.ID, c.Scope, escapePipes(c.Title), c.Status, c.Severity, escapePipes(c.Analysis), escapePipes(c.Fix))
	}
	b.WriteString("\n")

	if len(r.Frames) > 0 {
		b.WriteString("## 对照帧\n\n")
		for _, f := range r.Frames {
			fmt.Fprintf(&b, "### %s\n\n", f.Role)
			fmt.Fprintf(&b, "%s  \n", f.Description)
			fmt.Fprintf(&b, "`%s` — %dx%d, sha256:`%s`\n\n", f.File, f.Width, f.Height, shortHash(f.SHA256))
			fmt.Fprintf(&b, "![%s](%s)\n\n", f.Role, f.File)
		}
	}

	if len(r.RepairCommands) > 0 {
		b.WriteString("## 可执行修复/复测命令\n\n```bash\n")
		for _, command := range r.RepairCommands {
			b.WriteString(command + "\n")
		}
		b.WriteString("```\n")
	}
	if len(r.NextSteps) > 0 {
		b.WriteString("\n## 建议下一步\n\n")
		for _, step := range r.NextSteps {
			fmt.Fprintf(&b, "- %s\n", step)
		}
	}
	return b.String()
}

func valueOrDash(s string) string {
	if strings.TrimSpace(s) == "" {
		return "-"
	}
	return s
}

func shortHash(h string) string {
	if len(h) <= 12 {
		return h
	}
	return h[:12]
}

func escapePipes(s string) string {
	return strings.ReplaceAll(s, "|", "\\|")
}
