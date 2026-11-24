package runner_test

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"batchproc/internal/engine"
	"batchproc/internal/model"
	"batchproc/internal/runner"
	"batchproc/internal/store"
)

type testEnv struct {
	t      *testing.T
	layout *store.Layout
}

func newEnv(t *testing.T) *testEnv {
	t.Helper()
	layout, err := store.NewLayout(filepath.Join(t.TempDir(), "out"))
	if err != nil {
		t.Fatalf("layout: %v", err)
	}
	return &testEnv{t: t, layout: layout}
}

func (e *testEnv) config(jobs ...model.Job) runner.Config {
	eng := engine.New()
	return runner.Config{
		Jobs:   jobs,
		Layout: e.layout,
		Engine: eng,
		Out:    io.Discard,
	}
}

func crashingEngine(stage model.StageName) *engine.Engine {
	eng := engine.New()
	eng.Kill = func(jobID string, got model.StageName, written int64) error {
		return engine.ErrSimulatedCrash
	}
	_ = stage
	return eng
}

func findTempFiles(t *testing.T, root string) []string {
	t.Helper()
	var temps []string
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		if strings.HasPrefix(d.Name(), ".tmp-") {
			temps = append(temps, path)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk: %v", err)
	}
	return temps
}

func TestCrashResumeAndIdempotent(t *testing.T) {
	env := newEnv(t)
	job := model.Job{ID: "j1", SizeKB: 128}

	cfg1 := env.config(job)
	cfg1.Engine = crashingEngine(model.StageEncode)
	cfg1.Crash = engine.CrashRule{JobID: "j1", Stage: model.StageEncode}
	_, err := runner.Run(cfg1)
	if !errors.Is(err, engine.ErrSimulatedCrash) {
		t.Fatalf("expected simulated crash, got %v", err)
	}

	if _, statErr := os.Stat(env.layout.FinalPath(job)); !os.IsNotExist(statErr) {
		t.Fatalf("final artifact must not exist after crash, stat=%v", statErr)
	}
	if temps := findTempFiles(t, env.layout.MetaDir); len(temps) == 0 {
		t.Fatal("expected a leftover .tmp-* part after hard crash")
	}

	var state model.JobState
	if err := store.ReadJSON(env.layout.StatePath("j1"), &state); err != nil {
		t.Fatalf("read state: %v", err)
	}
	if state.Stages["prepare"] == nil || !state.Stages["prepare"].Done ||
		state.Stages["render"] == nil || !state.Stages["render"].Done {
		t.Fatalf("prepare/render checkpoints should be committed: %+v", state.Stages)
	}
	if state.Stages["encode"] != nil {
		t.Fatal("crashed encode stage must not be checkpointed")
	}

	cfg2 := env.config(job)
	sum2, err := runner.Run(cfg2)
	if err != nil {
		t.Fatalf("resume run: %v", err)
	}
	if sum2.Completed != 1 {
		t.Fatalf("expected 1 completed, got %+v", sum2)
	}
	res := sum2.Results[0]
	if strings.Join(res.StagesRun, ",") != "encode,publish" {
		t.Fatalf("resume should run encode/publish, got %v", res.StagesRun)
	}
	if strings.Join(res.StagesSkipped, ",") != "prepare,render" {
		t.Fatalf("resume should skip prepare/render, got %v", res.StagesSkipped)
	}
	if st := runner.InspectArtifact(env.layout, job); !st.MarkerValid {
		t.Fatalf("marker should be valid after resume: %+v", st)
	}
	if temps := findTempFiles(t, env.layout.OutDir); len(temps) != 0 {
		t.Fatalf("temp parts should be swept/renamed, found %v", temps)
	}

	cfg3 := env.config(job)
	sum3, err := runner.Run(cfg3)
	if err != nil {
		t.Fatalf("idempotent run: %v", err)
	}
	if sum3.Skipped != 1 || sum3.Completed != 0 {
		t.Fatalf("duplicate execution should skip, got %+v", sum3)
	}
}

func TestTamperedCheckpointArtifactCascades(t *testing.T) {
	env := newEnv(t)
	job := model.Job{ID: "j2", SizeKB: 64}

	cfg1 := env.config(job)
	cfg1.Engine = crashingEngine(model.StageEncode)
	cfg1.Crash = engine.CrashRule{JobID: "j2", Stage: model.StageEncode}
	_, _ = runner.Run(cfg1)

	renderArtifact := env.layout.StageArtifact("j2", model.StageRender)
	if err := os.WriteFile(renderArtifact, []byte("tampered"), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg2 := env.config(job)
	sum, err := runner.Run(cfg2)
	if err != nil {
		t.Fatalf("resume: %v", err)
	}
	res := sum.Results[0]
	if strings.Join(res.StagesSkipped, ",") != "prepare" {
		t.Fatalf("only prepare may be skipped, got skipped=%v run=%v",
			res.StagesSkipped, res.StagesRun)
	}
	if strings.Join(res.StagesRun, ",") != "render,encode,publish" {
		t.Fatalf("tamper should cascade from render, got %v", res.StagesRun)
	}
}
