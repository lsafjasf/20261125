package runner_test

import (
	"errors"
	"io"
	"os"
	"strings"
	"testing"

	"batchproc/internal/confirm"
	"batchproc/internal/model"
	"batchproc/internal/runner"
	"batchproc/internal/store"
)

func TestSingleJobFailureIsolated(t *testing.T) {
	env := newEnv(t)
	good1 := model.Job{ID: "g1", SizeKB: 32}
	bad := model.Job{ID: "bad", SizeKB: 32, FailStage: "render"}
	good2 := model.Job{ID: "g2", SizeKB: 32}

	sum, err := runner.Run(env.config(good1, bad, good2))
	if err != nil {
		t.Fatalf("batch should not return a fatal error for task failure: %v", err)
	}
	if sum.Completed != 2 || sum.Failed != 1 {
		t.Fatalf("expected completed=2 failed=1, got %+v", sum)
	}
	if !runner.InspectArtifact(env.layout, good1).MarkerValid {
		t.Error("good1 artifact should be valid")
	}
	if !runner.InspectArtifact(env.layout, good2).MarkerValid {
		t.Error("good2 artifact should be valid even though a middle job failed")
	}
	if _, err := os.Stat(env.layout.FinalPath(bad)); !os.IsNotExist(err) {
		t.Error("failed job must not publish an artifact")
	}

	var state model.JobState
	if err := readState(env, "bad", &state); err != nil {
		t.Fatal(err)
	}
	if state.Status != model.StatusFailed || state.LastError == "" {
		t.Fatalf("failed state should persist error: %+v", state)
	}
}

func TestOverwriteProtection(t *testing.T) {
	env := newEnv(t)
	job := model.Job{ID: "ow", SizeKB: 32}
	if _, err := runner.Run(env.config(job)); err != nil {
		t.Fatal(err)
	}
	before := runner.InspectArtifact(env.layout, job)

	runForce := func(stdin string, yes bool) (*runner.Summary, error) {
		cfg := env.config(job)
		cfg.Force = true
		cfg.Prompter = &confirm.Prompter{
			In:        strings.NewReader(stdin),
			Out:       io.Discard,
			AssumeYes: yes,
		}
		return runner.Run(cfg)
	}

	if _, err := runForce("", false); !errors.Is(err, runner.ErrOverwriteConfirmation) {
		t.Fatalf("empty stdin must require confirmation, got %v", err)
	}
	if after := runner.InspectArtifact(env.layout, job); after.ActualDigest != before.ActualDigest {
		t.Fatal("artifact changed after EOF confirmation refusal")
	}

	if _, err := runForce("maybe\n", false); !errors.Is(err, runner.ErrOverwriteConfirmation) {
		t.Fatalf("non-yes answer must abort, got %v", err)
	}
	if after := runner.InspectArtifact(env.layout, job); after.ActualDigest != before.ActualDigest {
		t.Fatal("artifact changed after explicit decline")
	}

	sum, err := runForce("yes\n", false)
	if err != nil {
		t.Fatalf("typed yes should allow overwrite: %v", err)
	}
	if len(sum.Results[0].StagesRun) != 4 {
		t.Fatalf("confirmed force should rerun all stages, got %v", sum.Results[0].StagesRun)
	}

	// --yes is the scriptable form of explicit confirmation.
	sum2, err := runForce("", true)
	if err != nil {
		t.Fatalf("--yes should authorize overwrite: %v", err)
	}
	if len(sum2.Results[0].StagesRun) != 4 {
		t.Fatalf("--yes should rerun all stages, got %v", sum2.Results[0].StagesRun)
	}
}

func TestTruncatedFinalIsQuarantinedAndRepublished(t *testing.T) {
	env := newEnv(t)
	job := model.Job{ID: "trunc", SizeKB: 32}
	finalPath := env.layout.FinalPath(job)
	if err := os.WriteFile(finalPath, []byte("this is a truncated legacy render"), 0o644); err != nil {
		t.Fatal(err)
	}

	sum, err := runner.Run(env.config(job))
	if err != nil || sum.Completed != 1 {
		t.Fatalf("run should recover truncated file: sum=%+v err=%v", sum, err)
	}
	if !runner.InspectArtifact(env.layout, job).MarkerValid {
		t.Fatal("republished artifact should have a valid marker")
	}
	entries, err := os.ReadDir(env.layout.QuarantineDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected exactly one quarantined file, got %d", len(entries))
	}
}

func TestMissingMarkerIsRepaired(t *testing.T) {
	env := newEnv(t)
	job := model.Job{ID: "mark", SizeKB: 32}
	if _, err := runner.Run(env.config(job)); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(env.layout.MarkerPath(env.layout.FinalPath(job))); err != nil {
		t.Fatal(err)
	}

	sum, err := runner.Run(env.config(job))
	if err != nil {
		t.Fatal(err)
	}
	res := sum.Results[0]
	if res.Outcome != runner.OutcomeCompleted || len(res.StagesRun) != 0 {
		t.Fatalf("marker repair should run zero stages, outcome=%s run=%v",
			res.Outcome, res.StagesRun)
	}
	if !runner.InspectArtifact(env.layout, job).MarkerValid {
		t.Fatal("marker should be restored")
	}
}

func readState(env *testEnv, id string, v any) error {
	return store.ReadJSON(env.layout.StatePath(id), v)
}
