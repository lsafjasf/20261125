package demo

import (
	"path/filepath"

	"batchproc/internal/model"
	"batchproc/internal/runner"
	"batchproc/internal/store"
)

func (d *demo) scenarioIsolation() error {
	d.section("场景 C：单任务确定性失败 —— 不拖垮整批")
	dir := d.resetScenario("C-isolation")
	out := filepath.Join(dir, "out")
	jobsPath := filepath.Join(dir, "jobs.json")
	jobs := []model.Job{
		{ID: "good-a", SizeKB: 512},
		{ID: "bad-b", SizeKB: 512, FailStage: "render"},
		{ID: "good-c", SizeKB: 512},
	}
	if err := writeJobsFile(jobsPath, jobs...); err != nil {
		return err
	}

	res := d.run("run", "--jobs", jobsPath, "--out", out,
		"--run-id", "c1", "--chunk-delay", "1ms")

	events := loadEvents(out)
	m := metricsForRun(events, "c1")
	layout, _ := store.NewLayout(out)
	goodA := runner.InspectArtifact(layout, jobs[0])
	goodC := runner.InspectArtifact(layout, jobs[2])
	badFinal := fileExists(layout.FinalPath(jobs[1]))

	d.checkf("部分失败时批处理退出码为 2", res.code == 2, "code=%d", res.code)
	d.checkf("前后两个正常任务都成功", m.JobDone == 2 && goodA.MarkerValid && goodC.MarkerValid,
		"jobDone=%d, goodA=%v, goodC=%v", m.JobDone, goodA.MarkerValid, goodC.MarkerValid)
	d.checkf("坏任务被记录为失败", m.JobFailed == 1, "jobFailed=%d", m.JobFailed)
	d.checkf("坏任务没有产物", !badFinal, "bad final exists=%v", badFinal)

	d.rows = append(d.rows, tableRow{"单任务失败影响",
		"一个任务报错，整批中断",
		"错误被捕获并隔离，其余任务照常完成，退出码 2"})
	return nil
}

func (d *demo) scenarioOverwrite() error {
	d.section("场景 D：成功产物保护 —— 未显式确认不得覆盖")
	dir := d.resetScenario("D-overwrite")
	out := filepath.Join(dir, "out")
	jobsPath := filepath.Join(dir, "jobs.json")
	job := model.Job{ID: "clip-d", SizeKB: 512}
	if err := writeJobsFile(jobsPath, job); err != nil {
		return err
	}
	layout, _ := store.NewLayout(out)
	finalPath := layout.FinalPath(job)

	first := d.run("run", "--jobs", jobsPath, "--out", out, "--run-id", "d0")
	before := runner.InspectArtifact(layout, job)

	noTTY := d.run("run", "--jobs", jobsPath, "--out", out, "--force", "--run-id", "d1")
	afterNoTTY := runner.InspectArtifact(layout, job)

	declined := d.runWithStdin("no\n", "run", "--jobs", jobsPath, "--out", out,
		"--force", "--run-id", "d2")
	afterDecline := runner.InspectArtifact(layout, job)

	confirmed := d.runWithStdin("yes\n", "run", "--jobs", jobsPath, "--out", out,
		"--force", "--run-id", "d3")
	afterYes := runner.InspectArtifact(layout, job)

	events := loadEvents(out)
	m3 := metricsForRun(events, "d3")

	d.checkf("首次渲染成功", first.code == 0 && before.MarkerValid, "code=%d", first.code)
	d.checkf("非交互且无 --yes 时拒绝覆盖(退出码3)", noTTY.code == 3, "code=%d", noTTY.code)
	d.checkf("空确认后 SHA-256 不变", before.ActualDigest == afterNoTTY.ActualDigest,
		"before=%s after=%s", shortDigest(before.ActualDigest), shortDigest(afterNoTTY.ActualDigest))
	d.checkf("输入 no 时拒绝覆盖(退出码3)", declined.code == 3, "code=%d", declined.code)
	d.checkf("拒绝后 SHA-256 仍不变", before.ActualDigest == afterDecline.ActualDigest,
		"same=%v", before.ActualDigest == afterDecline.ActualDigest)
	d.checkf("输入完整 yes 后才覆盖重跑", confirmed.code == 0 && len(m3.StageDone) == 4,
		"code=%d stages=%v", confirmed.code, m3.StageDone)
	d.checkf("覆盖后新产物标记有效", afterYes.MarkerValid, "markerValid=%v", afterYes.MarkerValid)

	d.rows = append(d.rows, tableRow{"成功产物覆盖策略",
		"os.Create 立即截断，旧产物无法挽回",
		"默认跳过；--force 须输入 yes 或传 --yes，临时文件原子替换"})
	_ = finalPath
	return nil
}

func shortDigest(s string) string {
	if len(s) <= 12 {
		return s
	}
	return s[:12]
}
