package main

import (
	"fmt"
	"path/filepath"
	"strings"
)

type frameAdder func(role, path, description string) error

func writeCorrectedFrames(cfg cliConfig, o Observation, rawPath string, addFrame frameAdder) (string, string, error) {
	candidatePaths := make([]string, 0, 4)
	for turns := 0; turns < 4; turns++ {
		img, err := loadImage(rawPath)
		if err != nil {
			return "", "", err
		}
		path := filepath.Join(cfg.outDir, fmt.Sprintf("candidate-raw-cw%d.png", turns*90))
		if err := savePNG(path, rotateTurns(img, turns)); err != nil {
			return "", "", err
		}
		candidatePaths = append(candidatePaths, path)
		if err := addFrame(
			fmt.Sprintf("raw-candidate-cw%d", turns*90),
			path,
			fmt.Sprintf("No-autorotate pixels rotated exactly %d degrees clockwise; visually choose the upright candidate.", turns*90),
		); err != nil {
			return "", "", err
		}
	}

	if o.Pixel.CorrectionTurnsCW != nil {
		turns := *o.Pixel.CorrectionTurnsCW
		img, err := loadImage(rawPath)
		if err != nil {
			return "", "", err
		}
		path := filepath.Join(cfg.outDir, "04-corrected.png")
		if err := savePNG(path, rotateTurns(img, turns)); err != nil {
			return "", "", err
		}
		desc := fmt.Sprintf("Corrected from raw pixels with exactly %d clockwise degree(s), source=%s.", turns*90, o.Pixel.Source)
		if err := addFrame("corrected", path, desc); err != nil {
			return "", "", err
		}
		return path, desc, nil
	}

	img, err := loadImage(rawPath)
	if err != nil {
		return "", "", err
	}
	path := filepath.Join(cfg.outDir, "04-matrix-applied-unverified.png")
	if err := savePNG(path, rotateTurns(img, o.MatrixTurnsCW)); err != nil {
		return "", "", err
	}
	desc := "Matrix-derived candidate only; semantic upright orientation is unverified. Compare candidate-raw-cw*.png or rerun with -reference/-pixel-correction-cw."
	if err := addFrame("correction-unverified", path, desc); err != nil {
		return "", "", err
	}
	return path, desc, nil
}

func buildConclusion(o Observation, combos []ComboCheck) string {
	fileHigh := HighRiskFileCombos(combos)
	if len(fileHigh) > 0 {
		ids := make([]string, 0, len(fileHigh))
		for _, c := range fileHigh {
			ids = append(ids, c.ID)
		}
		return fmt.Sprintf("检测到 %d 个高风险文件元数据/像素组合（%s）。必须以 -noautorotate 原始像素为准，并用参考图或显式方向确定唯一修正；不要在默认 autorotate 的结果上再次 transpose。",
			len(fileHigh), strings.Join(ids, ", "))
	}
	for _, c := range combos {
		if c.Status == "needs_visual_check" || c.Status == "orientation_unverified" {
			return "几何元数据存在需要目视确认的旋转组合，但仅凭宽高和矩阵不能证明语义上的二次旋转；请查看原始帧、矩阵播放器帧和四张 raw 旋转候选，或提供正向参考图。C3 还复现了默认 autorotate 后再手动旋转的管线二次旋转。"
		}
	}
	if o.MatrixActive {
		return "发现非单位旋转矩阵，但未发现已证实的文件内二次旋转；矩阵感知播放器与忽略矩阵的播放器仍可能不一致，且不要在默认 autorotate 后再手动 transpose。"
	}
	return "未发现非单位旋转矩阵；播放器差异更可能来自显示宽高/SAR、裁剪或解码器，而不是显示矩阵二次旋转。"
}

func buildNextSteps(cfg cliConfig, o Observation) []string {
	steps := []string{
		"先打开 01-raw-pixels-noautorotate.png，确认编码像素网格的真实方向；不要用播放器窗口尺寸代替它。",
		"再打开 02-matrix-respecting-player.png，它代表遵循 Display Matrix 的播放器/FFmpeg 默认 autorotate 结果。",
		"检查 rotation-report.md 中 C1-C5 的状态；高风险项表示已证实或高度疑似二次旋转。",
	}
	if o.Pixel.CorrectionTurnsCW == nil {
		steps = append(steps, "语义正向未知：目视选择 candidate-raw-cw0/90/180/270.png，或用 -reference upright.png / -pixel-correction-cw 0|90|180|270 重跑。")
	} else if o.ResidualTurnsCW != nil && *o.ResidualTurnsCW == 0 {
		steps = append(steps, "矩阵与真实像素所需方向一致：播放链路只应用矩阵一次；FFmpeg 抽普通帧即可，不要再加 transpose。")
	} else {
		steps = append(steps, "以 04-corrected.png 为准；修复文件时要么清除/改写 tkhd 矩阵，要么烘入唯一旋转后写单位矩阵，不能两者同时保留。")
	}
	return steps
}

func buildRepairCommands(cfg cliConfig, o Observation, at string) []string {
	quotedInput := shellQuote(cfg.input)
	seek := ""
	if at != "" {
		seek = "-ss " + at + " "
	}
	commands := []string{
		fmt.Sprintf("%s -v error -show_streams -show_format -of json %s", cfg.ffprobeBin, quotedInput),
		fmt.Sprintf("%s -hide_banner -loglevel error -noautorotate %s-i %s -frames:v 1 raw-pixels.png", cfg.ffmpegBin, seek, quotedInput),
		fmt.Sprintf("%s -hide_banner -loglevel error %s-i %s -frames:v 1 matrix-player.png", cfg.ffmpegBin, seek, quotedInput),
	}
	if o.Pixel.CorrectionTurnsCW != nil {
		turns := *o.Pixel.CorrectionTurnsCW
		filter := transposeChain(turns)
		frameCmd := fmt.Sprintf("%s -hide_banner -loglevel error -noautorotate %s-i %s -frames:v 1", cfg.ffmpegBin, seek, quotedInput)
		videoCmd := fmt.Sprintf("%s -hide_banner -loglevel error -noautorotate -i %s", cfg.ffmpegBin, quotedInput)
		if filter != "" {
			frameCmd += " -vf " + shellQuote(filter)
			videoCmd += " -vf " + shellQuote(filter)
		}
		commands = append(commands,
			frameCmd+" corrected-frame.png",
			videoCmd+" -c:v libx264 -pix_fmt yuv420p -c:a copy corrected-baked.mp4",
		)
	}
	commands = append(commands, "# 禁止：默认 autorotate 抽帧后再按 ffprobe rotation 手动 transpose；这会复现 03-double-rotated PNG。")
	return commands
}

func transposeChain(turnsCW int) string {
	switch normTurnsCW(turnsCW) {
	case 1:
		return "transpose=1"
	case 2:
		return "hflip,vflip"
	case 3:
		return "transpose=2"
	default:
		return ""
	}
}

func shellQuote(value string) string {
	if value == "" {
		return "''"
	}
	return "'" + strings.ReplaceAll(value, "'", `'\''`) + "'"
}
