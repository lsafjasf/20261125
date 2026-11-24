package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"time"

	"batchproc/internal/confirm"
	"batchproc/internal/engine"
	"batchproc/internal/model"
	"batchproc/internal/runner"
	"batchproc/internal/store"
)

func cmdRun(args []string) int {
	fs := flag.NewFlagSet("run", flag.ContinueOnError)
	jobsPath := fs.String("jobs", "jobs.json", "任务定义 JSON")
	outDir := fs.String("out", "./out", "产物输出目录")
	force := fs.Bool("force", false, "允许重跑已有成功标记的任务（仍需显式确认）")
	yes := fs.Bool("yes", false, "以参数形式给出显式覆盖确认（脚本场景）")
	crashSpec := fs.String("crash", "", "注入硬崩溃 job-id:stage")
	runID := fs.String("run-id", "", "本次运行 ID（事件日志用）")
	chunkDelay := fs.Duration("chunk-delay", 0, "每个写入块的模拟耗时")
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
	id := *runID
	if id == "" {
		id = fmt.Sprintf("run-%d", time.Now().UnixNano())
	}
	events, err := store.NewEventLogger(layout.EventsPath, id)
	if err != nil {
		fmt.Fprintf(os.Stderr, "打开事件日志失败: %v\n", err)
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

	fmt.Printf("批处理启动 run_id=%s，任务数=%d，输出=%s\n", id, len(jobs), layout.OutDir)
	summary, runErr := runner.Run(runner.Config{
		Jobs:     jobs,
		Layout:   layout,
		Events:   events,
		Engine:   eng,
		Prompter: &confirm.Prompter{In: os.Stdin, Out: os.Stdout, AssumeYes: *yes},
		Out:      os.Stdout,
		Force:    *force,
		Crash:    rule,
	})

	if runErr != nil {
		switch {
		case errors.Is(runErr, runner.ErrOverwriteConfirmation):
			fmt.Fprintf(os.Stderr, "\n已中止: %v\n", runErr)
			return 3
		case errors.Is(runErr, engine.ErrSimulatedCrash):
			return 137
		default:
			fmt.Fprintf(os.Stderr, "批处理异常: %v\n", runErr)
			return 1
		}
	}

	fmt.Printf("\n批次汇总: 成功=%d 幂等跳过=%d 失败=%d\n",
		summary.Completed, summary.Skipped, summary.Failed)
	for _, r := range summary.Results {
		if r.Outcome == runner.OutcomeFailed && r.Err != nil {
			fmt.Fprintf(os.Stderr, "  失败任务 %s: %v\n", r.JobID, r.Err)
		}
	}
	if summary.Failed > 0 {
		return 2
	}
	return 0
}

// hardKill is the production crash hook: it terminates immediately without
// defers, closely matching SIGKILL semantics for on-disk state.
func hardKill(jobID string, stage model.StageName, written int64) error {
	fmt.Fprintf(os.Stderr,
		"💥 [注入崩溃] job=%s stage=%s 已写 %d 字节；立即退出(137)，未提交阶段状态\n",
		jobID, stage, written)
	_ = os.Stderr.Sync()
	os.Exit(137)
	return engine.ErrSimulatedCrash
}
