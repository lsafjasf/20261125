package main

import (
	"bufio"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"time"
)

type options struct {
	file    string
	mode    string
	mapping string
	names   string
	backup  string
	yes     bool
	dryRun  bool
}

func main() {
	if err := run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr, time.Now); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func run(args []string, stdin io.Reader, stdout, stderr io.Writer, now func() time.Time) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: wavchan inspect|repair|rollback [flags]")
	}
	command := args[0]
	fs := flag.NewFlagSet(command, flag.ContinueOnError)
	fs.SetOutput(stderr)
	opts := options{}
	fs.StringVar(&opts.file, "file", "", "input WAV file")
	fs.StringVar(&opts.mode, "mode", "", "repair mode: samples or declaration")
	fs.StringVar(&opts.mapping, "mapping", "", "comma-separated source positions, 1-based")
	fs.StringVar(&opts.names, "names", "", "comma-separated ordered declaration names")
	fs.StringVar(&opts.backup, "backup", "", "backup file for rollback")
	fs.BoolVar(&opts.yes, "yes", false, "apply without confirmation")
	fs.BoolVar(&opts.dryRun, "dry-run", false, "show preview without writing")
	if err := fs.Parse(args[1:]); err != nil {
		return err
	}
	if opts.file == "" {
		return errors.New("--file is required")
	}

	switch command {
	case "inspect":
		return inspectCommand(opts.file, stdout)
	case "repair":
		return repairCommand(opts, stdin, stdout, now)
	case "rollback":
		return rollbackCommand(opts, stdout)
	default:
		return fmt.Errorf("unknown command %q", command)
	}
}

func inspectCommand(path string, out io.Writer) error {
	w, err := readWAVFile(path)
	if err != nil {
		return err
	}
	printEvidence(out, collectEvidence(w))
	return nil
}

func repairCommand(opts options, stdin io.Reader, out io.Writer, now func() time.Time) error {
	w, err := readWAVFile(opts.file)
	if err != nil {
		return err
	}
	evidence := collectEvidence(w)
	printEvidence(out, evidence)

	request, err := buildRepairRequest(opts, w, stdin, out)
	if err != nil {
		return err
	}
	result, err := repairWAV(w, request)
	if err != nil {
		return err
	}
	if err := printPreview(out, result); err != nil {
		return err
	}

	if opts.dryRun {
		fmt.Fprintln(out, "\nDry run: no file was written and no backup was created.")
		return nil
	}
	if !opts.yes {
		confirmed, err := confirm(stdin, out, "\n输入 yes 应用修复，其他内容取消: ")
		if err != nil {
			return err
		}
		if !confirmed {
			fmt.Fprintln(out, "已取消，未修改文件。")
			return nil
		}
	}

	backup, err := createBackup(opts.file, now())
	if err != nil {
		return err
	}
	if err := atomicWrite(opts.file, result.output); err != nil {
		return err
	}
	hash, err := readback(opts.file, result.output)
	if err != nil {
		fmt.Fprintf(out, "回读失败；备份仍位于 %s\n", backup.path)
		return err
	}
	printReadback(out, backup, hash, result)
	return nil
}

func buildRepairRequest(opts options, w *wavFile, stdin io.Reader, out io.Writer) (repairRequest, error) {
	channels := int(w.format.channels)
	mode := repairMode(opts.mode)
	if mode == "" {
		if !interactive(stdin) {
			return repairRequest{}, fmt.Errorf("non-interactive repair requires --mode")
		}
		fmt.Fprintln(out, "\n选择修复方式，只能二选一:")
		fmt.Fprintln(out, "  1. samples: 重排未压缩 PCM 样本，保留 fmt/cmap 声明")
		fmt.Fprintln(out, "  2. declaration: 更新 cmap 声明，PCM 字节保持不变")
		choice, err := readLine(stdin, out, "输入 1 或 2: ")
		if err != nil {
			return repairRequest{}, err
		}
		switch strings.TrimSpace(choice) {
		case "1":
			mode = modeSamples
		case "2":
			mode = modeDeclaration
		default:
			return repairRequest{}, fmt.Errorf("invalid mode choice %q", choice)
		}
	}
	if mode != modeSamples && mode != modeDeclaration {
		return repairRequest{}, fmt.Errorf("invalid mode %q", opts.mode)
	}

	if mode == modeSamples {
		mapping, err := chooseMapping(opts, w, stdin, out)
		if err != nil {
			return repairRequest{}, err
		}
		return repairRequest{mode: mode, mapping: mapping}, nil
	}

	names, err := declarationNames(w, opts.names, stdin, out)
	if err != nil {
		return repairRequest{}, err
	}
	return repairRequest{mode: mode, mapping: identityMapping(channels), names: names}, nil
}

func chooseMapping(opts options, w *wavFile, stdin io.Reader, out io.Writer) ([]int, error) {
	channels := int(w.format.channels)
	if opts.mapping != "" {
		return parseMapping(opts.mapping, channels)
	}
	if !interactive(stdin) {
		return nil, fmt.Errorf("non-interactive samples repair requires --mapping")
	}
	fmt.Fprintf(out, "\n选择实际样本到输出声道的映射（%d 个 1-based 源位置，逗号分隔）。\n", channels)
	fmt.Fprintln(out, "当前声明位置:", strings.Join(w.declaredNames(), ", "))
	candidates := candidateMappings(channels)
	printMappingCandidates(out, w.declaredNames())
	text, err := readLine(stdin, out, "输入候选编号，或直接输入逗号分隔映射；回车保持原顺序: ")
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(text) == "" {
		return identityMapping(channels), nil
	}
	var choice int
	if _, err := fmt.Sscanf(strings.TrimSpace(text), "%d", &choice); err == nil &&
		choice >= 1 && choice <= len(candidates) && strings.TrimSpace(text) == fmt.Sprintf("%d", choice) {
		return candidates[choice-1], nil
	}
	return parseMapping(text, channels)
}

func declarationNames(w *wavFile, override string, stdin io.Reader, out io.Writer) ([]string, error) {
	if override != "" {
		return parseNames(override, int(w.format.channels))
	}
	if !interactive(stdin) {
		return nil, fmt.Errorf("non-interactive declaration repair requires --names")
	}
	names := w.declaredNames()
	fmt.Fprintf(out, "\n声明模式将写入新的 cmap 顺序；当前顺序: %s\n", strings.Join(names, ", "))
	candidates := candidateMappings(len(names))
	printMappingCandidates(out, names)
	text, err := readLine(stdin, out, "输入候选编号，或逗号分隔的新顺序名称: ")
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(text) == "" {
		return nil, errors.New("declaration mode requires a new ordered declaration")
	}
	var choice int
	if _, err := fmt.Sscanf(strings.TrimSpace(text), "%d", &choice); err == nil &&
		choice >= 1 && choice <= len(candidates) && strings.TrimSpace(text) == fmt.Sprintf("%d", choice) {
		selected := make([]string, len(names))
		for outputPosition, sourcePosition := range candidates[choice-1] {
			selected[outputPosition] = names[sourcePosition]
		}
		return selected, nil
	}
	return parseNames(text, int(w.format.channels))
}

func rollbackCommand(opts options, out io.Writer) error {
	hash, err := rollback(opts.file, opts.backup)
	if err != nil {
		return err
	}
	fmt.Fprintf(out, "回滚完成，目标文件 SHA-256: %s\n", hash)
	return nil
}

func interactive(reader io.Reader) bool {
	file, ok := reader.(*os.File)
	if !ok {
		return false
	}
	info, err := file.Stat()
	return err == nil && info.Mode()&os.ModeCharDevice != 0
}

func confirm(reader io.Reader, out io.Writer, prompt string) (bool, error) {
	text, err := readLine(reader, out, prompt)
	if err != nil {
		return false, err
	}
	return strings.EqualFold(strings.TrimSpace(text), "yes"), nil
}

func readLine(reader io.Reader, out io.Writer, prompt string) (string, error) {
	fmt.Fprint(out, prompt)
	scanner := bufio.NewScanner(reader)
	if !scanner.Scan() {
		if scanner.Err() != nil {
			return "", scanner.Err()
		}
		return "", io.EOF
	}
	return scanner.Text(), nil
}
