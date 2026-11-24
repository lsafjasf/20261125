package server

import (
	"bytes"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"math"
	"mime"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"forensicaudio/internal/analysis"
	"forensicaudio/internal/audio"
	"forensicaudio/internal/demo"
)

//go:embed static/*
var assets embed.FS

const (
	maxUploadBytes = 256 << 20
	maxDurationSec = 15 * 60
	defaultSens    = 0.55
)

type Server struct {
	mux      *http.ServeMux
	sessions *store
}

func New() *Server {
	s := &Server{mux: http.NewServeMux(), sessions: newStore(2 * time.Hour)}
	staticFS, err := fs.Sub(assets, "static")
	if err != nil {
		panic(err)
	}
	s.mux.HandleFunc("GET /", s.index)
	s.mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServer(http.FS(staticFS))))
	s.mux.HandleFunc("GET /api/health", s.health)
	s.mux.HandleFunc("POST /api/demo", s.createDemo)
	s.mux.HandleFunc("POST /api/sessions", s.createSession)
	s.mux.HandleFunc("GET /api/sessions/{id}", s.getSession)
	s.mux.HandleFunc("GET /api/sessions/{id}/original", s.originalAudio)
	s.mux.HandleFunc("GET /api/sessions/{id}/waveform", s.waveform)
	s.mux.HandleFunc("POST /api/sessions/{id}/sensitivity", s.updateSensitivity)
	return s
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Referrer-Policy", "no-referrer")
	s.mux.ServeHTTP(w, r)
}

func (s *Server) index(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	page, err := assets.ReadFile("static/index.html")
	if err != nil {
		http.Error(w, "page unavailable", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Security-Policy", "default-src 'self'; img-src 'self' data:; media-src 'self'; style-src 'self' 'unsafe-inline'; script-src 'self'; connect-src 'self'")
	_, _ = w.Write(page)
}

func (s *Server) health(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "time": time.Now().UTC()})
}

func (s *Server) createDemo(w http.ResponseWriter, r *http.Request) {
	raw, err := demo.Generate()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	s.ingest(w, raw, "forensic-demo.wav")
}

func (s *Server) createSession(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxUploadBytes)
	raw, filename, err := readUpload(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	s.ingest(w, raw, filename)
}

func (s *Server) ingest(w http.ResponseWriter, raw []byte, filename string) {
	wav, err := audio.Parse(raw)
	if err != nil {
		writeError(w, http.StatusUnsupportedMediaType, err)
		return
	}
	if wav.Duration > maxDurationSec {
		writeError(w, http.StatusRequestEntityTooLarge,
			fmt.Errorf("audio duration %.1fs exceeds the %ds limit", wav.Duration, maxDurationSec))
		return
	}
	result, err := analysis.Analyze(wav, filename, defaultSens)
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, err)
		return
	}
	sess, err := s.sessions.add(filename, wav, result)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{
		"id":          sess.id,
		"analysis":    sess.analysis,
		"audioUrl":    "/api/sessions/" + sess.id + "/original",
		"waveformUrl": "/api/sessions/" + sess.id + "/waveform",
	})
}

func readUpload(r *http.Request) ([]byte, string, error) {
	mediaType, _, _ := mime.ParseMediaType(r.Header.Get("Content-Type"))
	filename := filepath.Base(strings.TrimSpace(r.URL.Query().Get("name")))
	if filename == "" || filename == "." {
		filename = "upload.wav"
	}
	if mediaType == "multipart/form-data" {
		if err := r.ParseMultipartForm(32 << 20); err != nil {
			return nil, "", fmt.Errorf("parse multipart form: %w", err)
		}
		if r.MultipartForm != nil {
			defer r.MultipartForm.RemoveAll()
		}
		file, header, err := r.FormFile("audio")
		if err != nil {
			return nil, "", errors.New("multipart field 'audio' is required")
		}
		defer file.Close()
		if header.Filename != "" {
			filename = filepath.Base(header.Filename)
		}
		raw, err := io.ReadAll(file)
		if err != nil {
			return nil, "", err
		}
		if len(raw) == 0 {
			return nil, "", errors.New("empty upload")
		}
		return raw, filename, nil
	}
	raw, err := io.ReadAll(r.Body)
	if err != nil {
		return nil, "", err
	}
	if len(raw) == 0 {
		return nil, "", errors.New("request body is empty")
	}
	return raw, filename, nil
}

func (s *Server) getSession(w http.ResponseWriter, r *http.Request) {
	sess, err := s.sessions.get(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusNotFound, err)
		return
	}
	sess.mu.RLock()
	defer sess.mu.RUnlock()
	writeJSON(w, http.StatusOK, sess.analysis)
}

func (s *Server) originalAudio(w http.ResponseWriter, r *http.Request) {
	sess, err := s.sessions.get(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusNotFound, err)
		return
	}
	sess.mu.RLock()
	original := sess.audio.Original
	filename := sess.filename
	hash := sess.audio.SHA256
	created := sess.created
	sess.mu.RUnlock()
	w.Header().Set("Content-Type", "audio/wav")
	w.Header().Set("Content-Disposition", fmt.Sprintf("inline; filename=%q", filename))
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Original-Sha256", hash)
	w.Header().Set("X-Audio-Processing", "none")
	http.ServeContent(w, r, filename, created, bytes.NewReader(original))
}

func (s *Server) updateSensitivity(w http.ResponseWriter, r *http.Request) {
	sess, err := s.sessions.get(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusNotFound, err)
		return
	}
	var req struct {
		Sensitivity *float64 `json:"sensitivity"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, errors.New("invalid JSON body"))
		return
	}
	if req.Sensitivity == nil || math.IsNaN(*req.Sensitivity) || math.IsInf(*req.Sensitivity, 0) ||
		*req.Sensitivity < 0 || *req.Sensitivity > 1 {
		writeError(w, http.StatusBadRequest, errors.New("sensitivity must be between 0 and 1"))
		return
	}
	sess.mu.Lock()
	rev := sess.analysis.SetSensitivity(*req.Sensitivity)
	resp := struct {
		Sensitivity float64                `json:"sensitivity"`
		Threshold   analysis.ThresholdInfo `json:"threshold"`
		Stats       analysis.Stats         `json:"stats"`
		Alignment   analysis.Alignment     `json:"alignment"`
		Revision    analysis.Revision      `json:"revision"`
	}{
		Sensitivity: sess.analysis.Sensitivity,
		Threshold:   sess.analysis.Threshold,
		Stats:       sess.analysis.Stats,
		Alignment:   sess.analysis.Alignment,
		Revision:    rev,
	}
	sess.mu.Unlock()
	writeJSON(w, http.StatusOK, resp)
}

type waveformResponse struct {
	Buckets  int       `json:"buckets"`
	Duration float64   `json:"duration"`
	Peaks    []float64 `json:"peaks"`
}

func (s *Server) waveform(w http.ResponseWriter, r *http.Request) {
	sess, err := s.sessions.get(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusNotFound, err)
		return
	}
	buckets := 2400
	if v := r.URL.Query().Get("buckets"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			buckets = n
		}
	}
	if buckets < 200 {
		buckets = 200
	}
	if buckets > 12000 {
		buckets = 12000
	}
	sess.mu.RLock()
	wav := sess.audio
	sess.mu.RUnlock()
	peaks := make([]float64, buckets)
	if wav.FrameCount > 0 {
		for b := 0; b < buckets; b++ {
			start := int(int64(b) * int64(wav.FrameCount) / int64(buckets))
			end := int(int64(b+1) * int64(wav.FrameCount) / int64(buckets))
			if end <= start {
				end = start + 1
			}
			if end > wav.FrameCount {
				end = wav.FrameCount
			}
			peak := 0.0
			for i := start; i < end; i++ {
				for ch := 0; ch < wav.Channels; ch++ {
					v := math.Abs(wav.Interleaved[i*wav.Channels+ch])
					if v > peak {
						peak = v
					}
				}
			}
			peaks[b] = math.Round(peak*10000) / 10000
		}
	}
	writeJSON(w, http.StatusOK, waveformResponse{
		Buckets:  buckets,
		Duration: math.Round(wav.Duration*10000) / 10000,
		Peaks:    peaks,
	})
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(value)
}

func writeError(w http.ResponseWriter, status int, err error) {
	writeJSON(w, status, map[string]string{"error": err.Error()})
}
