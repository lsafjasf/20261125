package demo

import (
	"io"
	"os"
	"path/filepath"
	"strconv"

	"batchproc/internal/model"
	"batchproc/internal/runner"
	"batchproc/internal/store"
)

const demoSizeKB = "2048"

func (d *demo) scenarioNaive() error {
	d.section("场景 A：旧批处理器 —— 崩溃留下截断文件，重启从头渲染")
	dir := d.resetScenario("A-naive")
	out := filepath.Join(dir, "out")

	crash := d.run("__naive", "--out", out, "--id", "clip-a",
		"--size-kb", demoSizeKB, "--crash", "clip-a:encode",
		"--run-id", "a1", "--chunk-delay", "1ms")

	final := filepath.Join(out, "clip-a.mp4")
	truncatedSize := fileSize(final)
	markerAfterCrash := fileExists(final + ".ok")
	if truncatedSize > 0 {
		_ = copyFile(final, filepath.Join(dir, "truncated-evidence.mp4"))
	}

	restart := d.run("__naive", "--out", out, "--id", "clip-a",
		"--size-kb", demoSizeKB, "--run-id", "a2", "--chunk-delay", "1ms")
	finalSize := fileSize(final)

	events := loadEvents(out)
	m1 := metricsForRun(events, "a1")
	m2 := metricsForRun(events, "a2")

	d.checkf("旧逻辑崩溃退出码为 137", crash.code == 137, "code=%d", crash.code)
	d.checkf("旧逻辑崩溃后留下非空截断文件", truncatedSize > 0,
		"clip-a.mp4=%d 字节", maxInt64(truncatedSize, 0))
	d.checkf("旧逻辑没有完成标记", !markerAfterCrash, "marker exists=%v", markerAfterCrash)
	d.checkf("旧逻辑重启后 4 个阶段全部重跑", restart.code == 0 && len(m2.StageDone) == 4,
		"exit=%d, a2 stages=%v", restart.code, m2.StageDone)
	d.checkf("截断文件小于完整产物", truncatedSize > 0 && finalSize > truncatedSize,
		"truncated=%d, full=%d", truncatedSize, finalSize)

	d.rows = append(d.rows,
		tableRow{"崩溃后正式产物",
			"clip-a.mp4 被截断 (" + humanSize(truncatedSize) + ")",
			"正式产物不存在，字节只进 .tmp-*"},
		tableRow{"完成标记 / 校验",
			"无 .ok、无 SHA-256",
			"未完成不写标记，完成后写 .ok"},
		tableRow{"重启后实际执行阶段",
			strconv.Itoa(len(m2.StageDone)) + "/4 阶段全部重跑，0 跳过",
			"仅 encode/publish 重跑；prepare/render 检查点跳过"},
		tableRow{"崩溃当次已做工作",
			strconv.Itoa(len(m1.StageDone)) + " 个阶段完成但无状态可恢复",
			"prepare/render 已落检查点"},
	)
	return nil
}

func (d *demo) scenarioResume() error {
	d.section("场景 B：修复后 —— 同一崩溃，重启从检查点恢复；再次执行幂等")
	dir := d.resetScenario("B-safe")
	out := filepath.Join(dir, "out")
	jobsPath := filepath.Join(dir, "jobs.json")
	if err := writeJobsFile(jobsPath, model.Job{ID: "clip-b", SizeKB: 2048}); err != nil {
		return err
	}

	first := d.run("run", "--jobs", jobsPath, "--out", out,
		"--crash", "clip-b:encode", "--run-id", "b1", "--chunk-delay", "1ms")

	final := filepath.Join(out, "clip-b.mp4")
	finalAfterCrash := fileExists(final)
	tempLeft := countTempParts(out)

	resume := d.run("run", "--jobs", jobsPath, "--out", out,
		"--run-id", "b2", "--chunk-delay", "1ms")
	tempAfterResume := countTempParts(out)

	verify := d.run("verify", "--jobs", jobsPath, "--out", out)

	again := d.run("run", "--jobs", jobsPath, "--out", out,
		"--run-id", "b3", "--chunk-delay", "1ms")

	events := loadEvents(out)
	m2 := metricsForRun(events, "b2")
	m3 := metricsForRun(events, "b3")

	layout, _ := store.NewLayout(out)
	status := runner.InspectArtifact(layout, model.Job{ID: "clip-b"})

	d.checkf("修复版崩溃退出码为 137", first.code == 137, "code=%d", first.code)
	d.checkf("崩溃后不存在截断的正式产物", !finalAfterCrash, "final exists=%v", finalAfterCrash)
	d.checkf("崩溃后留有临时残片", tempLeft > 0, ".tmp files=%d", tempLeft)
	d.checkf("断点恢复退出码为 0", resume.code == 0, "code=%d", resume.code)
	d.checkf("恢复时只执行 encode/publish", len(m2.StageDone) == 2 &&
		m2.StageDone[0] == "encode" && m2.StageDone[1] == "publish",
		"done=%v", m2.StageDone)
	d.checkf("prepare/render 命中检查点跳过", len(m2.StageSkip) == 2, "skip=%v", m2.StageSkip)
	d.checkf("重启后清扫了临时残片", tempAfterResume == 0, "remaining .tmp=%d", tempAfterResume)
	d.checkf("verify 校验通过", verify.code == 0 && status.MarkerValid,
		"verify=%d markerValid=%v", verify.code, status.MarkerValid)
	d.checkf("重复执行零阶段渲染且跳过任务", again.code == 0 &&
		len(m3.StageDone) == 0 && m3.JobSkipped == 1,
		"b3 done=%v jobSkip=%d", m3.StageDone, m3.JobSkipped)

	d.rows = append(d.rows,
		tableRow{"崩溃残留与清扫",
			"正式文件本身是残片，只能再次截断重写",
			"留 " + strconv.Itoa(tempLeft) + " 个 .tmp-*，重启清扫，残留 " +
				strconv.Itoa(tempAfterResume)},
		tableRow{"重复执行（第三次启动）",
			"无状态，仍会 " + demoSizeKB + " KiB 全量重渲染",
			"0 个阶段执行，识别有效 .ok 后 uptodate 跳过"},
		tableRow{"校验命令 verify",
			"无法判断文件是否完整",
			"大小 + SHA-256 + 标记三者一致才 OK"},
	)
	return nil
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()
	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return out.Sync()
}

func maxInt64(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}

func humanSize(n int64) string {
	if n < 0 {
		return "不存在"
	}
	if n < 1024 {
		return strconv.FormatInt(n, 10) + " B"
	}
	if n < 1024*1024 {
		return strconv.FormatFloat(float64(n)/1024, 'f', 1, 64) + " KiB"
	}
	return strconv.FormatFloat(float64(n)/(1024*1024), 'f', 2, 64) + " MiB"
}
