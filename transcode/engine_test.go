package transcode

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
)

func writeTestInput(t *testing.T, dir, name string, data []byte) string {
	t.Helper()
	file := filepath.Join(dir, name)
	if err := os.WriteFile(file, data, 0o644); err != nil {
		t.Fatal(err)
	}
	return file
}

func copyStage(name, primary string, calls *int) Stage {
	return Stage{
		Name:    name,
		Version: "1",
		Primary: primary,
		Run: func(ctx context.Context, input StageInput) error {
			*calls = *calls + 1
			data, err := os.ReadFile(input.InputPath)
			if err != nil {
				return err
			}
			return os.WriteFile(filepath.Join(input.OutputDir, primary), data, 0o644)
		},
	}
}

func completionCount(t *testing.T, root string) int {
	t.Helper()
	count := 0
	err := filepath.WalkDir(filepath.Join(root, "jobs"), func(name string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !entry.IsDir() {
			count++
		}
		return nil
	})
	if err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	return count
}

func TestRunIsContentAddressedAndIdempotent(t *testing.T) {
	root := t.TempDir()
	inputDir := t.TempDir()
	content := []byte("the same source bytes")
	firstInput := writeTestInput(t, inputDir, "first.bin", content)
	secondInput := writeTestInput(t, inputDir, "second.bin", content)

	engine := Engine{Root: root, Pipeline: PassthroughPipeline()}
	first, err := engine.Run(context.Background(), Request{InputPath: firstInput})
	if err != nil {
		t.Fatal(err)
	}
	if first.Reused {
		t.Fatal("first run unexpectedly reused output")
	}
	got, err := os.ReadFile(first.PrimaryPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(content) {
		t.Fatalf("primary output = %q, want %q", got, content)
	}

	second, err := engine.Run(context.Background(), Request{InputPath: secondInput})
	if err != nil {
		t.Fatal(err)
	}
	if !second.Reused {
		t.Fatal("same content and parameters should reuse the output")
	}
	if second.Key != first.Key || second.ObjectDigest != first.ObjectDigest ||
		second.PrimaryPath != first.PrimaryPath {
		t.Fatalf("result changed across identical runs:\nfirst:  %#v\nsecond: %#v", first, second)
	}
}

func TestKeyChangesWithParametersAndStageVersion(t *testing.T) {
	root := t.TempDir()
	input := writeTestInput(t, t.TempDir(), "input.bin", []byte("payload"))
	calls := 0
	versionOne := copyStage("render", "output.bin", &calls)
	engine := Engine{Root: root, Pipeline: []Stage{versionOne}}
	first, err := engine.Run(context.Background(), Request{InputPath: input})
	if err != nil {
		t.Fatal(err)
	}
	withParameter, err := engine.Run(context.Background(), Request{
		InputPath:  input,
		Parameters: map[string]string{"quality": "high"},
	})
	if err != nil {
		t.Fatal(err)
	}
	versionTwo := versionOne
	versionTwo.Version = "2"
	engine.Pipeline = []Stage{versionTwo}
	withVersion, err := engine.Run(context.Background(), Request{InputPath: input})
	if err != nil {
		t.Fatal(err)
	}
	if first.Key == withParameter.Key {
		t.Fatal("parameter change did not change the job key")
	}
	if first.Key == withVersion.Key {
		t.Fatal("stage version change did not change the job key")
	}
}

func TestConcurrentRunsShareOneCompletedObject(t *testing.T) {
	root := t.TempDir()
	input := writeTestInput(t, t.TempDir(), "input.bin", []byte("payload"))
	var calls atomic.Int32
	engine := Engine{
		Root: root,
		Pipeline: []Stage{{
			Name:    "render",
			Version: "1",
			Primary: "output.bin",
			Run: func(ctx context.Context, input StageInput) error {
				calls.Add(1)
				return os.WriteFile(filepath.Join(input.OutputDir, "output.bin"), []byte("done"), 0o644)
			},
		}},
	}
	results := make([]Result, 2)
	errs := make([]error, 2)
	var group sync.WaitGroup
	for index := range results {
		group.Add(1)
		go func(index int) {
			defer group.Done()
			results[index], errs[index] = engine.Run(context.Background(), Request{InputPath: input})
		}(index)
	}
	group.Wait()
	for index, err := range errs {
		if err != nil {
			t.Fatalf("run %d: %v", index, err)
		}
	}
	if calls.Load() != 1 {
		t.Fatalf("stage ran %d times, want 1", calls.Load())
	}
	if results[0].ObjectDigest != results[1].ObjectDigest {
		t.Fatal("concurrent runs produced different objects")
	}
	if count := completionCount(t, root); count != 1 {
		t.Fatalf("completion markers = %d, want 1", count)
	}
}

func TestFailedStageIsNotCheckpointedAndResumes(t *testing.T) {
	root := t.TempDir()
	input := writeTestInput(t, t.TempDir(), "input.bin", []byte("payload"))
	injected := errors.New("injected render failure")
	inspectCalls := 0
	renderCalls := 0
	pipeline := []Stage{
		{
			Name:    "inspect",
			Version: "1",
			Primary: "first.txt",
			Run: func(ctx context.Context, input StageInput) error {
				inspectCalls++
				return os.WriteFile(filepath.Join(input.OutputDir, "first.txt"), []byte("first"), 0o644)
			},
		},
		{
			Name:    "render",
			Version: "1",
			Primary: "second.txt",
			Run: func(ctx context.Context, input StageInput) error {
				renderCalls++
				if renderCalls == 1 {
					if err := os.WriteFile(filepath.Join(input.OutputDir, "partial.txt"), []byte("half"), 0o644); err != nil {
						return err
					}
					return injected
				}
				if _, err := os.Stat(filepath.Join(input.OutputDir, "partial.txt")); err == nil {
					return errors.New("partial output leaked into retry")
				}
				return os.WriteFile(filepath.Join(input.OutputDir, "second.txt"), []byte("second"), 0o644)
			},
		},
	}
	engine := Engine{Root: root, Pipeline: pipeline}
	if _, err := engine.Run(context.Background(), Request{InputPath: input}); !errors.Is(err, injected) {
		t.Fatalf("first run error = %v, want injected failure", err)
	}
	if count := completionCount(t, root); count != 0 {
		t.Fatalf("failed run left %d completion marker(s)", count)
	}

	result, err := engine.Run(context.Background(), Request{InputPath: input})
	if err != nil {
		t.Fatal(err)
	}
	if inspectCalls != 1 {
		t.Fatalf("inspect stage ran %d times, want 1 committed run", inspectCalls)
	}
	if renderCalls != 2 {
		t.Fatalf("render stage ran %d times, want failure plus retry", renderCalls)
	}
	if _, err := os.Stat(filepath.Join(result.OutputDir, "stages", "render", "partial.txt")); !os.IsNotExist(err) {
		t.Fatal("partial artifact was published")
	}
}

func TestCorruptObjectIsRebuiltNotTrusted(t *testing.T) {
	root := t.TempDir()
	input := writeTestInput(t, t.TempDir(), "input.bin", []byte("payload"))
	calls := 0
	engine := Engine{Root: root, Pipeline: []Stage{copyStage("render", "output.bin", &calls)}}
	first, err := engine.Run(context.Background(), Request{InputPath: input})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(first.PrimaryPath, []byte("cut-off"), 0o644); err != nil {
		t.Fatal(err)
	}

	second, err := engine.Run(context.Background(), Request{InputPath: input})
	if err != nil {
		t.Fatal(err)
	}
	if second.Reused {
		t.Fatal("corrupt object was treated as complete")
	}
	got, err := os.ReadFile(second.PrimaryPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "payload" {
		t.Fatalf("repaired output = %q", got)
	}
	if second.ObjectDigest != first.ObjectDigest {
		t.Fatalf("repair changed deterministic digest: %s != %s", second.ObjectDigest, first.ObjectDigest)
	}
}

func TestExtraFileInObjectIsNotConsideredComplete(t *testing.T) {
	root := t.TempDir()
	input := writeTestInput(t, t.TempDir(), "input.bin", []byte("payload"))
	calls := 0
	engine := Engine{Root: root, Pipeline: []Stage{copyStage("render", "output.bin", &calls)}}
	first, err := engine.Run(context.Background(), Request{InputPath: input})
	if err != nil {
		t.Fatal(err)
	}
	extra := filepath.Join(first.OutputDir, "stages", "render", "partial.bin")
	if err := os.WriteFile(extra, []byte("half"), 0o644); err != nil {
		t.Fatal(err)
	}

	second, err := engine.Run(context.Background(), Request{InputPath: input})
	if err != nil {
		t.Fatal(err)
	}
	if second.Reused {
		t.Fatal("object with an unexplained partial file was reused")
	}
	if _, err := os.Stat(extra); !os.IsNotExist(err) {
		t.Fatal("partial file survived object repair")
	}
}

func TestInvalidCheckpointRerunsItAndDownstream(t *testing.T) {
	root := t.TempDir()
	input := writeTestInput(t, t.TempDir(), "input.bin", []byte("payload"))
	inspectCalls := 0
	renderCalls := 0
	pipeline := []Stage{
		copyStage("inspect", "first.bin", &inspectCalls),
		copyStage("render", "second.bin", &renderCalls),
	}
	engine := Engine{Root: root, Pipeline: pipeline}
	result, err := engine.Run(context.Background(), Request{InputPath: input})
	if err != nil {
		t.Fatal(err)
	}
	job := &preparedJob{layout: layout{root: root}, key: result.Key, stages: normalizeForTest(t, pipeline)}
	stageFile := filepath.Join(job.stageDir(0), "first.bin")
	if err := os.WriteFile(stageFile, []byte("corrupt"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(job.layout.jobMarker(job.key)); err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(result.OutputDir); err != nil {
		t.Fatal(err)
	}

	if _, err := engine.Run(context.Background(), Request{InputPath: input}); err != nil {
		t.Fatal(err)
	}
	if inspectCalls != 2 || renderCalls != 2 {
		t.Fatalf("stage calls = inspect:%d render:%d, want both rerun once", inspectCalls, renderCalls)
	}
}

func normalizeForTest(t *testing.T, pipeline []Stage) []Stage {
	t.Helper()
	stages, _, err := normalizePipeline(pipeline)
	if err != nil {
		t.Fatal(err)
	}
	return stages
}
