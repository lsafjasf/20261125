package main

import (
	"bytes"
	"fmt"
	"io"
	"math"
	"strings"
)

func printEvidence(out io.Writer, e evidence) {
	fmt.Fprintf(out, "检测证据\n")
	fmt.Fprintf(out, "  文件大小: %d bytes\n", e.fileSize)
	fmt.Fprintf(out, "  文件 SHA-256: %s\n", e.fileHash)
	fmt.Fprintf(out, "  RIFF 大小: 声明=%d 实际=%d 一致=%t\n", e.riffSize, e.fileSize-8, e.riffSizeOK)
	fmt.Fprintf(out, "  PCM SHA-256: %s\n", e.dataHash)
	fmt.Fprintf(out, "  格式: audioFormat=%d channels=%d sampleRate=%d bits=%d\n",
		e.format.audioFormat, e.format.channels, e.format.sampleRate, e.format.bitsPerSample)
	fmt.Fprintf(out, "  帧/块: frames=%d blockAlign=%d sampleBytes=%d\n",
		e.frameCount, e.blockAlign, e.bytesPerSample)
	if e.format.hasMask {
		fmt.Fprintf(out, "  fmt 声道掩码: 0x%08X，位数一致=%t\n", e.format.mask, e.maskConsistent)
	}
	fmt.Fprintf(out, "  cmap 存在且一致: %t\n", e.cmapConsistent)
	fmt.Fprintf(out, "  声明顺序: %s\n", strings.Join(e.declaredNames, ", "))
	fmt.Fprintln(out)
	fmt.Fprintln(out, "  Chunks:")
	for _, c := range e.chunks {
		fmt.Fprintf(out, "    %-4s offset=%-6d size=%d\n", c.id, c.offset, c.size)
	}
	fmt.Fprintln(out)
	fmt.Fprintln(out, "  每声道证据:")
	fmt.Fprintln(out, "    POS DECLARED   FIRST    MIN      MAX      NONZERO RMS        HASH")
	for i, fp := range e.fingerprints {
		rms := 0.0
		if e.frameCount > 0 {
			rms = math.Sqrt(fp.sumSquares / float64(e.frameCount))
		}
		fmt.Fprintf(out, "    %3d %-9s %8d %8d %8d %7d %10.2f %s\n",
			i+1, fp.name, fp.first, fp.min, fp.max, fp.nonZero, rms, shortHash(fp.hash))
	}
}

func printPreview(out io.Writer, result *repairResult) error {
	repaired, err := parseWAV(bytes.NewReader(result.output))
	if err != nil {
		return err
	}
	fmt.Fprintf(out, "\n修复预览（互斥模式: %s）\n", result.mode)
	fmt.Fprintf(out, "  映射（输出位置 <- 源位置，1 起）: %s\n", joinMapping(result.mapping))
	fmt.Fprintf(out, "  修复前首帧: %v\n", result.firstBefore)
	fmt.Fprintf(out, "  修复后首帧: %v\n", result.firstAfter)
	fmt.Fprintf(out, "  PCM 数据哈希变化: %t\n", result.dataChanged)
	fmt.Fprintf(out, "    before: %s\n", result.beforeData)
	fmt.Fprintf(out, "    after:  %s\n", result.afterData)
	fmt.Fprintf(out, "  fmt 变化: %t; cmap 变化: %t\n", result.fmtChanged, result.cmapChanged)
	fmt.Fprintf(out, "  新声明顺序: %s\n", strings.Join(result.newNames, ", "))
	fmt.Fprintf(out, "  输出文件 SHA-256: %s\n", result.afterFile)

	limit := 3
	if repaired.frameCount() < limit {
		limit = repaired.frameCount()
	}
	fmt.Fprintf(out, "  前 %d 帧样本预览:\n", limit)
	for frame := 0; frame < limit; frame++ {
		fmt.Fprintf(out, "    frame %d: %v\n", frame+1, frameSamples(repaired, frame))
	}
	return nil
}

func joinMapping(mapping []int) string {
	parts := make([]string, len(mapping))
	for i, value := range mapping {
		parts[i] = fmt.Sprintf("%d<-%d", i+1, value+1)
	}
	return strings.Join(parts, " ")
}

func frameSamples(w *wavFile, frame int) []int64 {
	channels := int(w.format.channels)
	bytesPerSample := int(w.format.bitsPerSample) / 8
	dataStart := w.data.offset + 8
	values := make([]int64, channels)
	for channel := 0; channel < channels; channel++ {
		start := dataStart + (frame*channels+channel)*bytesPerSample
		values[channel] = readPCM(w.raw[start : start+bytesPerSample])
	}
	return values
}

func shortHash(value string) string {
	if len(value) < 12 {
		return value
	}
	return value[:12] + "…"
}

func printReadback(out io.Writer, backup backupInfo, readbackHash string, result *repairResult) {
	fmt.Fprintf(out, "\n回读结果\n")
	fmt.Fprintf(out, "  目标文件读回 SHA-256: %s\n", readbackHash)
	fmt.Fprintf(out, "  与预期输出一致: %t\n", readbackHash == result.afterFile)
	fmt.Fprintf(out, "  备份路径: %s\n", backup.path)
	fmt.Fprintf(out, "  备份 SHA-256: %s\n", backup.hash)
	fmt.Fprintf(out, "  备份大小: %d bytes\n", backup.size)
	fmt.Fprintf(out, "  回滚命令: wavchan rollback --file <目标文件> --backup %s\n", backup.path)
}
