package server

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"sync"
	"time"

	"forensicaudio/internal/analysis"
	"forensicaudio/internal/audio"
)

type session struct {
	mu       sync.RWMutex
	id       string
	filename string
	audio    *audio.WAV
	analysis *analysis.Analysis
	created  time.Time
}

type store struct {
	mu       sync.RWMutex
	sessions map[string]*session
	ttl      time.Duration
}

func newStore(ttl time.Duration) *store {
	return &store{sessions: make(map[string]*session), ttl: ttl}
}

func (s *store) add(filename string, w *audio.WAV, result *analysis.Analysis) (*session, error) {
	id, err := newID()
	if err != nil {
		return nil, err
	}
	result.ID = id
	sess := &session{
		id:       id,
		filename: filename,
		audio:    w,
		analysis: result,
		created:  time.Now(),
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	cutoff := time.Now().Add(-s.ttl)
	for key, item := range s.sessions {
		if item.created.Before(cutoff) {
			delete(s.sessions, key)
		}
	}
	s.sessions[id] = sess
	return sess, nil
}

func (s *store) get(id string) (*session, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	sess, ok := s.sessions[id]
	if !ok {
		return nil, errors.New("session not found")
	}
	return sess, nil
}

func newID() (string, error) {
	var buf [12]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf[:]), nil
}
