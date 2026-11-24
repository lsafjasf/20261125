// Package demo runs an end-to-end before/after comparison. It re-executes
// the same Go binary as subprocesses so that crashes and restarts are real
// process boundaries rather than in-process simulations.
package demo

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"batchproc/internal/model"
	"batchproc/internal/store"
)

type procResult struct {
	args   []string
	code   int
	output string
}

// runBinary executes the current single binary with explicit stdin.
func runBinary(bin, dir, stdin string, args ...string) procResult {
	cmd := exec.Command(bin, args...)
	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf
	if stdin != "" {
		cmd.Stdin = strings.NewReader(stdin)
	}
	if dir != "" {
		cmd.Dir = dir
	}
	err := cmd.Run()
	res := procResult{args: args, output: buf.String()}
	if err == nil {
		res.code = 0
		return res
	}
	if exitErr, ok := err.(*exec.ExitError); ok {
		res.code = exitErr.ExitCode()
		return res
	}
	res.code = -1
	res.output += "\n" + err.Error()
	return res
}

type runMetrics struct {
	StageDone  []string
	StageSkip  []string
	JobDone    int
	JobFailed  int
	JobSkipped int
	Crash      int
}

func metricsForRun(events []model.Event, runID string) runMetrics {
	var m runMetrics
	for _, e := range events {
		if e.RunID != runID {
			continue
		}
		switch e.Kind {
		case model.EventStageDone:
			m.StageDone = append(m.StageDone, e.Stage)
		case model.EventStageSkip:
			m.StageSkip = append(m.StageSkip, e.Stage)
		case model.EventJobDone:
			m.JobDone++
		case model.EventJobFailed:
			m.JobFailed++
		case model.EventJobSkip:
			m.JobSkipped++
		case model.EventCrash:
			m.Crash++
		}
	}
	return m
}

func loadEvents(outDir string) []model.Event {
	l, err := store.NewLayout(outDir)
	if err != nil {
		return nil
	}
	events, err := store.ReadEvents(l.EventsPath)
	if err != nil {
		return nil
	}
	return events
}

func fileSize(path string) int64 {
	info, err := os.Stat(path)
	if err != nil {
		return -1
	}
	return info.Size()
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// countTempParts counts leftover atomic-write temp files (.tmp-*) anywhere
// under the output directory.
func countTempParts(root string) int {
	count := 0
	_ = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		if strings.HasPrefix(d.Name(), ".tmp-") {
			count++
		}
		return nil
	})
	return count
}

func writeJobsFile(path string, jobs ...model.Job) error {
	return store.WriteJSONAtomic(path, model.JobsFile{Jobs: jobs})
}

func indentOutput(output, prefix string) string {
	lines := strings.Split(strings.TrimRight(output, "\n"), "\n")
	var b strings.Builder
	for _, line := range lines {
		b.WriteString(prefix)
		b.WriteString(line)
		b.WriteByte('\n')
	}
	return b.String()
}
