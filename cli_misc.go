package main

import (
	"flag"
	"fmt"
	"os"
	"time"

	"batchproc/internal/demo"
	"batchproc/internal/engine"
	"batchproc/internal/model"
	"batchproc/internal/runner"
	"batchproc/internal/store"
)

func cmdVerify(args []string) int {
	fs := flag.NewFlagSet("verify", flag.ContinueOnError)
	jobsPath := fs.String("jobs", "jobs.json", "任务定义 JSON")
	outDir := fs.String("out", "./out", "产物输出目录")
	if err := fs.Parse(args); err != nil {
		return 1
	}
	jobs, err := runner.LoadJobs(*jobsPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "加载任务文件失败: %v\n", err)
		return 1
	}
	layout, err := store.NewLayout(*outDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "初始化输出目录失败: %v\n", err)
		return 1
	}

	reports, temps, err := runner.VerifyAll(layout, jobs)
	if err != nil {
		fmt.Fprintf(os.Stderr, "校验失败: %v\n", err)
		return 1
	}
	bad := 0
	for _, rep := range reports {
		if rep.OK {
			fmt.Printf("  ✓ %-12s %s  size=%d sha256=%s\n",
				rep.Job.ID, rep.Reason, rep.Status.ActualSize, short(rep.Status.ActualDigest))
		} else {
			bad++
			fmt.Printf("  ✗ %-12s %s\n", rep.Job.ID, rep.Reason)
		}
	}
	for _, tmp := range temps {
		fmt.Printf("  ⚠ 残留临时文件: %s\n", tmp)
	}
	if bad > 0 || len(temps) > 0 {
		fmt.Printf("\n校验: %d 个任务产物不完整，%d 个临时残片\n", bad, len(temps))
		return 1
	}
	fmt.Printf("\n校验通过: %d 个产物均有有效完成标记且 SHA-256 一致，无临时残片\n", len(reports))
	return 0
}

func cmdDemo(args []string) int {
	fs := flag.NewFlagSet("demo", flag.ContinueOnError)
	dir := fs.String("dir", "", "场景工作区根目录（默认临时目录）")
	keep := fs.Bool("keep", false, "保留临时工作区")
	report := fs.String("report", "", "Markdown 对照报告输出路径")
	if err := fs.Parse(args); err != nil {
		return 1
	}
	err := demo.Run(demo.Options{
		RootDir:    *dir,
		Keep:       *keep,
		ReportPath: *report,
		Out:        os.Stdout,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "演示断言失败: %v\n", err)
		return 1
	}
	return 0
}

func cmdNaive(args []string) int {
	fs := flag.NewFlagSet("__naive", flag.ContinueOnError)
	outDir := fs.String("out", "./out", "产物目录")
	id := fs.String("id", "", "任务 ID")
	sizeKB := fs.Int("size-kb", 2048, "渲染负载 KiB")
	crashSpec := fs.String("crash", "", "注入硬崩溃 job-id:stage")
	runID := fs.String("run-id", "", "运行 ID")
	chunkDelay := fs.Duration("chunk-delay", 0, "每块模拟耗时")
	if err := fs.Parse(args); err != nil {
		return 1
	}
	if *id == "" {
		fmt.Fprintln(os.Stderr, "__naive 需要 --id")
		return 1
	}
	layout, err := store.NewLayout(*outDir)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	rid := *runID
	if rid == "" {
		rid = fmt.Sprintf("naive-%d", time.Now().UnixNano())
	}
	events, err := store.NewEventLogger(layout.EventsPath, rid)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	defer events.Close()

	rule, err := engine.ParseCrashSpec(*crashSpec)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	eng := engine.New()
	eng.ChunkDelay = *chunkDelay
	eng.Kill = hardKill

	job := model.Job{ID: *id, SizeKB: *sizeKB}
	if err := engine.RunNaive(engine.NaiveConfig{
		Layout: layout, Events: events, Engine: eng, Job: job, Crash: rule,
	}); err != nil {
		if err == engine.ErrSimulatedCrash {
			return 137
		}
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	return 0
}

func short(digest string) string {
	if len(digest) > 12 {
		return digest[:12] + "…"
	}
	return digest
}
