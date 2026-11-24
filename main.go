package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"

	"ffv1mkaudit/audit"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		var usage *usageError
		if errors.As(err, &usage) {
			if errors.Is(err, flag.ErrHelp) {
				os.Exit(0)
			}
			fmt.Fprintln(os.Stderr, "用法: ffv1mkaudit [选项] <file.mkv|file.mka>")
			fmt.Fprintln(os.Stderr, "使用 -h 查看完整选项。")
			os.Exit(2)
		}
		fmt.Fprintf(os.Stderr, "错误: %v\n", err)
		os.Exit(2)
	}
}

type usageError struct{ err error }

func (e *usageError) Error() string { return e.err.Error() }
func (e *usageError) Unwrap() error { return e.err }

func run(args []string) error {
	fs := flag.NewFlagSet("ffv1mkaudit", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	jsonOut := fs.Bool("json", false, "向标准输出写入完整 JSON 报告")
	outFile := fs.String("o", "", "将完整 JSON 报告写入文件")
	expectFile := fs.String("expect", "", "期望值清单 JSON（slice 数、哈希、位置、关键帧）")
	slices := fs.Int("slices", 0, "所有视频帧期望的 FFV1 slice 数")
	crcMode := fs.String("ffv1-crc", "auto", "FFV1 slice CRC 模式: auto|on|off")
	hashFile := fs.Bool("hash-file", false, "同时计算整个输入文件的 SHA-256")
	maxFrameMB := fs.Int64("max-frame-mb", 512, "单个 block/frame 的安全读取上限（MiB）")
	failOn := fs.String("fail-on", "error", "退出码策略: error|warning|none")
	quiet := fs.Bool("quiet", false, "不输出人类可读摘要")
	if err := fs.Parse(args); err != nil {
		return &usageError{err}
	}
	if fs.NArg() != 1 {
		return &usageError{fmt.Errorf("expected exactly one Matroska file")}
	}
	if *crcMode != "auto" && *crcMode != "on" && *crcMode != "off" {
		return &usageError{fmt.Errorf("invalid -ffv1-crc %q", *crcMode)}
	}
	if *failOn != "error" && *failOn != "warning" && *failOn != "none" {
		return &usageError{fmt.Errorf("invalid -fail-on %q", *failOn)}
	}

	opts := audit.Options{
		ExpectedSlices: *slices,
		HashFile:       *hashFile,
		FFV1CRC:        *crcMode,
		MaxFrameBytes:  *maxFrameMB << 20,
	}
	if *expectFile != "" {
		data, err := os.ReadFile(*expectFile)
		if err != nil {
			return err
		}
		var expected audit.Expectations
		if err := json.Unmarshal(data, &expected); err != nil {
			return fmt.Errorf("parse expectations: %w", err)
		}
		opts.Expectations = &expected
	}

	report, err := audit.AnalyzePath(fs.Arg(0), opts)
	if err != nil {
		return err
	}
	if *outFile != "" {
		data, err := json.MarshalIndent(report, "", "  ")
		if err != nil {
			return err
		}
		if err := os.WriteFile(*outFile, append(data, '\n'), 0o644); err != nil {
			return err
		}
	}
	if *jsonOut {
		encoder := json.NewEncoder(os.Stdout)
		encoder.SetIndent("", "  ")
		if err := encoder.Encode(report); err != nil {
			return err
		}
	} else if !*quiet {
		printHuman(report)
	}

	switch *failOn {
	case "none":
		return nil
	case "warning":
		if report.Summary.Errors > 0 || report.Summary.Warnings > 0 {
			os.Exit(1)
		}
	default:
		if report.Summary.Errors > 0 {
			os.Exit(1)
		}
	}
	return nil
}
