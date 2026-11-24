package transcode

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestCancellationLeavesNoCompletion(t *testing.T) {
	root := t.TempDir()
	input := writeTestInput(t, t.TempDir(), "input.bin", []byte("payload"))
	started := make(chan struct{})
	engine := Engine{
		Root: root,
		Pipeline: []Stage{{
			Name:    "render",
			Version: "1",
			Primary: "output.bin",
			Run: func(ctx context.Context, input StageInput) error {
				close(started)
				<-ctx.Done()
				return ctx.Err()
			},
		}},
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		_, err := engine.Run(ctx, Request{InputPath: input})
		done <- err
	}()
	<-started
	cancel()
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatalf("Run error = %v, want context.Canceled", err)
	}
	if count := completionCount(t, root); count != 0 {
		t.Fatalf("canceled run left %d completion marker(s)", count)
	}
}

func TestRunWithRetryRetriesFromFailedStage(t *testing.T) {
	root := t.TempDir()
	input := writeTestInput(t, t.TempDir(), "input.bin", []byte("payload"))
	attempts := 0
	engine := Engine{
		Root: root,
		Pipeline: []Stage{{
			Name:    "render",
			Version: "1",
			Primary: "output.bin",
			Run: func(ctx context.Context, input StageInput) error {
				attempts++
				if attempts == 1 {
					return errors.New("transient disk pressure")
				}
				return os.WriteFile(filepath.Join(input.OutputDir, "output.bin"), []byte("complete"), 0o644)
			},
		}},
	}
	result, err := engine.RunWithRetry(context.Background(), Request{InputPath: input}, RetryPolicy{
		MaxAttempts: 3,
		InitialWait: time.Millisecond,
		MaxWait:     2 * time.Millisecond,
	})
	if err != nil {
		t.Fatal(err)
	}
	if attempts != 2 {
		t.Fatalf("attempts = %d, want 2", attempts)
	}
	data, err := os.ReadFile(result.PrimaryPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "complete" {
		t.Fatalf("output = %q", data)
	}
}

func TestChangedOutputForSameJobIsRejected(t *testing.T) {
	root := t.TempDir()
	input := writeTestInput(t, t.TempDir(), "input.bin", []byte("payload"))
	counter := 0
	engine := Engine{
		Root: root,
		Pipeline: []Stage{{
			Name:    "render",
			Version: "1",
			Primary: "output.bin",
			Run: func(ctx context.Context, input StageInput) error {
				counter++
				data := []byte(fmt.Sprintf("nondeterministic-%d", counter))
				return os.WriteFile(filepath.Join(input.OutputDir, "output.bin"), data, 0o644)
			},
		}},
	}
	first, err := engine.Run(context.Background(), Request{InputPath: input})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(first.OutputDir); err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(filepath.Join(root, "work")); err != nil {
		t.Fatal(err)
	}

	_, err = engine.Run(context.Background(), Request{InputPath: input})
	if !errors.Is(err, ErrNondeterministic) {
		t.Fatalf("Run error = %v, want ErrNondeterministic", err)
	}
}
