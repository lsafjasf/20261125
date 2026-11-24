package store

import (
	"encoding/json"
	"os"
	"sync"
	"time"

	"batchproc/internal/model"
)

func timestampForName() string {
	return time.Now().Format("20060102-150405.000000000")
}

// EventLogger appends model.Event records as JSON Lines. It is safe for
// concurrent use within one process.
type EventLogger struct {
	mu    sync.Mutex
	f     *os.File
	RunID string
}

// NewEventLogger opens (or creates) the JSONL event log at path.
func NewEventLogger(path, runID string) (*EventLogger, error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return nil, err
	}
	return &EventLogger{f: f, RunID: runID}, nil
}

// Log appends one event, filling timestamp/run/pid defaults.
func (l *EventLogger) Log(e model.Event) {
	if l == nil || l.f == nil {
		return
	}
	if e.TS.IsZero() {
		e.TS = time.Now()
	}
	if e.RunID == "" {
		e.RunID = l.RunID
	}
	if e.PID == 0 {
		e.PID = os.Getpid()
	}
	b, err := json.Marshal(e)
	if err != nil {
		return
	}
	b = append(b, '\n')
	l.mu.Lock()
	defer l.mu.Unlock()
	_, _ = l.f.Write(b)
	_ = l.f.Sync()
}

// Close releases the underlying file.
func (l *EventLogger) Close() error {
	if l == nil || l.f == nil {
		return nil
	}
	return l.f.Close()
}

// ReadEvents loads every event from a JSONL path.
func ReadEvents(path string) ([]model.Event, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var events []model.Event
	for _, line := range splitLines(data) {
		if len(line) == 0 {
			continue
		}
		var e model.Event
		if err := json.Unmarshal(line, &e); err != nil {
			continue
		}
		events = append(events, e)
	}
	return events, nil
}

func splitLines(data []byte) [][]byte {
	var lines [][]byte
	start := 0
	for i, b := range data {
		if b == '\n' {
			lines = append(lines, data[start:i])
			start = i + 1
		}
	}
	if start < len(data) {
		lines = append(lines, data[start:])
	}
	return lines
}
