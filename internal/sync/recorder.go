package sync

import (
	"encoding/json"
	"io"
	"sync"
	"time"
)

// Recorder is a concurrency-safe stage position collector. Embed or pass it
// to capture, PTS, editing, render, and playback code paths.
type Recorder struct {
	mu      sync.Mutex
	records []Record
}

func NewRecorder() *Recorder {
	return &Recorder{}
}

// Observe records one generic media position for a stage/frame.
func (r *Recorder) Observe(stage Stage, frame int64, position time.Duration) {
	value := position.Nanoseconds()
	frameValue := frame
	r.append(Record{
		Stage:      stage,
		Frame:      &frameValue,
		WallNS:     time.Now().UnixNano(),
		PositionNS: &value,
	})
}

// ObserveAV records synchronized video and audio positions for one frame.
// Lip offset is analyzed as audio_position - video_position.
func (r *Recorder) ObserveAV(stage Stage, frame int64, video, audio time.Duration) {
	videoNS := video.Nanoseconds()
	audioNS := audio.Nanoseconds()
	frameValue := frame
	r.append(Record{
		Stage:      stage,
		Frame:      &frameValue,
		WallNS:     time.Now().UnixNano(),
		VideoPosNS: &videoNS,
		AudioPosNS: &audioNS,
	})
}

// ObserveOutput records a stable frame and its stage-specific output index.
func (r *Recorder) ObserveOutput(stage Stage, frame, outputFrame int64, position time.Duration) {
	value := position.Nanoseconds()
	frameValue := frame
	outputValue := outputFrame
	r.append(Record{
		Stage:       stage,
		Frame:       &frameValue,
		OutputFrame: &outputValue,
		WallNS:      time.Now().UnixNano(),
		PositionNS:  &value,
	})
}

// Append stores a fully specified record.
func (r *Recorder) Append(record Record) {
	if record.WallNS == 0 {
		record.WallNS = time.Now().UnixNano()
	}
	r.append(record)
}

func (r *Recorder) append(record Record) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.records = append(r.records, record)
}

// Records returns a copy so callers can analyze without locking the recorder.
func (r *Recorder) Records() []Record {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]Record(nil), r.records...)
}

// WriteJSONL writes all records as newline-delimited JSON.
func (r *Recorder) WriteJSONL(writer io.Writer) error {
	encoder := json.NewEncoder(writer)
	for _, record := range r.Records() {
		if err := encoder.Encode(record); err != nil {
			return err
		}
	}
	return nil
}
