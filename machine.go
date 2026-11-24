package hdrmeta

import (
	"reflect"
	"sort"
	"sync"
)

// StateMachine stores accepted metadata generations and resolves the metadata
// eligible for each actual display moment.
//
// The zero value is not ready for use; call New or NewMachine.
type StateMachine[P any] struct {
	mu sync.RWMutex

	nextGenerationID int64
	generations      []Generation[P]
	ordered          []Generation[P]
	byKey            map[updateKey]int64
	byID             map[int64]Generation[P]

	history       []FrameRecord[P]
	haveLastFrame bool
	lastFrame     Frame
	lastAdoptedID int64
	currentState  State
}

type updateKey struct {
	scene     int64
	frame     int64
	pts       Tick
	updatedAt int64
}

// New creates an empty metadata state machine with a typed parameter payload.
func New[P any]() *StateMachine[P] {
	return &StateMachine[P]{
		byKey: make(map[updateKey]int64),
		byID:  make(map[int64]Generation[P]),
	}
}

// Ingest accepts one metadata block.
//
// A repeated natural update key with identical parameters is idempotent. A
// changed payload with the same key is retained as a newer correction. Every
// other update receives a fresh generation, including reuse of a scene number.
func (m *StateMachine[P]) Ingest(update Metadata[P]) IngestResult[P] {
	m.mu.Lock()
	defer m.mu.Unlock()

	key := updateKey{
		scene:     update.Scene,
		frame:     update.FrameNumber,
		pts:       update.PTS,
		updatedAt: update.UpdatedAt,
	}

	if existingID, ok := m.byKey[key]; ok {
		existing := m.byID[existingID]
		if reflect.DeepEqual(existing.Parameters, update.Parameters) {
			return IngestResult[P]{Generation: existing, Duplicate: true}
		}

		generation := m.newGenerationLocked(update)
		m.byKey[key] = generation.ID
		m.insertOrderedLocked(generation)
		return IngestResult[P]{Generation: generation, Corrected: true}
	}

	generation := m.newGenerationLocked(update)
	m.byKey[key] = generation.ID
	m.insertOrderedLocked(generation)
	return IngestResult[P]{Generation: generation}
}

func (m *StateMachine[P]) newGenerationLocked(update Metadata[P]) Generation[P] {
	m.nextGenerationID++
	generation := Generation[P]{
		ID:       m.nextGenerationID,
		Metadata: update,
	}
	m.generations = append(m.generations, generation)
	m.byID[generation.ID] = generation
	return generation
}

func (m *StateMachine[P]) insertOrderedLocked(generation Generation[P]) {
	at := sort.Search(len(m.ordered), func(i int) bool {
		return compareGenerations(m.ordered[i], generation) > 0
	})
	m.ordered = append(m.ordered, Generation[P]{})
	copy(m.ordered[at+1:], m.ordered[at:])
	m.ordered[at] = generation
}

func compareGenerations[P any](a, b Generation[P]) int {
	switch {
	case a.PTS < b.PTS:
		return -1
	case a.PTS > b.PTS:
		return 1
	}
	switch {
	case a.UpdatedAt < b.UpdatedAt:
		return -1
	case a.UpdatedAt > b.UpdatedAt:
		return 1
	}
	switch {
	case a.ID < b.ID:
		return -1
	case a.ID > b.ID:
		return 1
	default:
		return 0
	}
}

// MetadataAt returns the newest generation whose activation PTS is at or
// before at. Future scene metadata is therefore never selected early.
func (m *StateMachine[P]) MetadataAt(at Tick) Snapshot[P] {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.snapshotAtLocked(at)
}

// Query is a descriptive alias for MetadataAt.
func (m *StateMachine[P]) Query(at Tick) Snapshot[P] {
	return m.MetadataAt(at)
}

func (m *StateMachine[P]) snapshotAtLocked(at Tick) Snapshot[P] {
	if len(m.ordered) == 0 {
		return Snapshot[P]{At: at, State: StateIdle}
	}

	eligible := sort.Search(len(m.ordered), func(i int) bool {
		return m.ordered[i].PTS > at
	})
	if eligible == 0 {
		return Snapshot[P]{At: at, State: StateWaiting}
	}

	generation := m.ordered[eligible-1]
	return Snapshot[P]{
		At:           at,
		Found:        true,
		State:        StateActive,
		GenerationID: generation.ID,
		Generation:   generation,
	}
}

// Present resolves metadata at frame.PTS and appends an immutable adoption
// record. Later ingestion never rewrites the returned frame's generation.
func (m *StateMachine[P]) Present(frame Frame) FrameRecord[P] {
	m.mu.Lock()
	defer m.mu.Unlock()

	snapshot := m.snapshotAtLocked(frame.PTS)
	gap := m.gapLocked(frame)

	adoption := AdoptionNone
	adoptedID := int64(0)
	if snapshot.Found {
		adoptedID = snapshot.Generation.ID
		if adoptedID == m.lastAdoptedID {
			adoption = AdoptionHeld
		} else {
			adoption = AdoptionNew
		}
	}

	record := FrameRecord[P]{
		AuditID:              int64(len(m.history) + 1),
		Frame:                frame,
		State:                snapshot.State,
		Adoption:             adoption,
		Found:                snapshot.Found,
		GenerationID:         adoptedID,
		PreviousGenerationID: m.lastAdoptedID,
		Generation:           snapshot.Generation,
		Gap:                  gap,
	}

	m.history = append(m.history, record)
	m.haveLastFrame = true
	m.lastFrame = frame
	m.lastAdoptedID = adoptedID
	m.currentState = snapshot.State
	return record
}

func (m *StateMachine[P]) gapLocked(frame Frame) FrameGap {
	if !m.haveLastFrame || frame.Number < 0 {
		return FrameGap{}
	}
	if frame.Number <= m.lastFrame.Number+1 {
		return FrameGap{
			PreviousFrame: m.lastFrame.Number,
			CurrentFrame:  frame.Number,
		}
	}

	from := m.lastFrame.Number + 1
	to := frame.Number - 1
	return FrameGap{
		PreviousFrame: m.lastFrame.Number,
		CurrentFrame:  frame.Number,
		From:          from,
		To:            to,
		Count:         to - from + 1,
	}
}

// PresentAll presents frames in slice order and returns their audit records.
func (m *StateMachine[P]) PresentAll(frames []Frame) []FrameRecord[P] {
	records := make([]FrameRecord[P], 0, len(frames))
	for _, frame := range frames {
		records = append(records, m.Present(frame))
	}
	return records
}

// History returns a copy of all frame adoption records in presentation order.
func (m *StateMachine[P]) History() []FrameRecord[P] {
	m.mu.RLock()
	defer m.mu.RUnlock()

	out := make([]FrameRecord[P], len(m.history))
	copy(out, m.history)
	return out
}

// Records is an alias for History.
func (m *StateMachine[P]) Records() []FrameRecord[P] {
	return m.History()
}

// Current returns the most recent presentation record.
func (m *StateMachine[P]) Current() (FrameRecord[P], bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if len(m.history) == 0 {
		return FrameRecord[P]{}, false
	}
	return m.history[len(m.history)-1], true
}

// State returns the state from the most recent presentation.
func (m *StateMachine[P]) State() State {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.currentState
}

// Generations returns all non-duplicate generations in acceptance order.
func (m *StateMachine[P]) Generations() []Generation[P] {
	m.mu.RLock()
	defer m.mu.RUnlock()

	out := make([]Generation[P], len(m.generations))
	copy(out, m.generations)
	return out
}

// Generation returns one machine-assigned generation.
func (m *StateMachine[P]) Generation(id int64) (Generation[P], bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	generation, ok := m.byID[id]
	return generation, ok
}

// ResolveStream ingests all updates and then presents all frames. It is a
// convenient deterministic entry point for code-constructed test material.
func ResolveStream[P any](updates []Metadata[P], frames []Frame) *StateMachine[P] {
	machine := New[P]()
	for _, update := range updates {
		machine.Ingest(update)
	}
	machine.PresentAll(frames)
	return machine
}
