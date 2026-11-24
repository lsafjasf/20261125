package server

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"math/rand"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"forensicaudio/internal/analysis"
	"forensicaudio/internal/audio"
	"forensicaudio/internal/demo"
)

type createResponse struct {
	ID          string            `json:"id"`
	Analysis    analysis.Analysis `json:"analysis"`
	AudioURL    string            `json:"audioUrl"`
	WaveformURL string            `json:"waveformUrl"`
}

func testServer(t *testing.T) *httptest.Server {
	t.Helper()
	ts := httptest.NewServer(New())
	t.Cleanup(ts.Close)
	return ts
}

func postJSON(t *testing.T, url, body string) *http.Response {
	t.Helper()
	req, err := http.NewRequest(http.MethodPost, url, strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	return resp
}

func createDemoSession(t *testing.T, ts *httptest.Server) createResponse {
	t.Helper()
	resp, err := http.Post(ts.URL+"/api/demo", "application/json", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		raw, _ := io.ReadAll(resp.Body)
		t.Fatalf("create demo: %d %s", resp.StatusCode, raw)
	}
	var created createResponse
	if err := json.NewDecoder(resp.Body).Decode(&created); err != nil {
		t.Fatal(err)
	}
	if created.ID == "" || len(created.Analysis.Intervals) == 0 {
		t.Fatalf("incomplete create response: %+v", created)
	}
	return created
}

func TestDemoAPIAndOriginalAudio(t *testing.T) {
	ts := testServer(t)
	created := createDemoSession(t, ts)

	resp, err := http.Get(ts.URL + created.AudioURL)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("original status %d", resp.StatusCode)
	}
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	if len(raw) != created.Analysis.Metadata.Bytes {
		t.Fatalf("original length %d, want %d", len(raw), created.Analysis.Metadata.Bytes)
	}
	sum := sha256.Sum256(raw)
	if got := hex.EncodeToString(sum[:]); got != created.Analysis.Metadata.SHA256 {
		t.Fatalf("original bytes changed: %s != %s", got, created.Analysis.Metadata.SHA256)
	}
	if resp.Header.Get("X-Audio-Processing") != "none" {
		t.Fatal("missing no-processing marker")
	}
	if resp.Header.Get("X-Original-Sha256") != created.Analysis.Metadata.SHA256 {
		t.Fatal("original hash header mismatch")
	}

	rangeReq, err := http.NewRequest(http.MethodGet, ts.URL+created.AudioURL, nil)
	if err != nil {
		t.Fatal(err)
	}
	rangeReq.Header.Set("Range", "bytes=16-31")
	rangeResp, err := http.DefaultClient.Do(rangeReq)
	if err != nil {
		t.Fatal(err)
	}
	defer rangeResp.Body.Close()
	part, _ := io.ReadAll(rangeResp.Body)
	if rangeResp.StatusCode != http.StatusPartialContent || !bytes.Equal(part, raw[16:32]) {
		t.Fatalf("range response is not byte-identical: %d", rangeResp.StatusCode)
	}
}

func TestWaveformEndpoint(t *testing.T) {
	ts := testServer(t)
	created := createDemoSession(t, ts)
	resp, err := http.Get(ts.URL + created.WaveformURL + "?buckets=321")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var wave waveformResponse
	if err := json.NewDecoder(resp.Body).Decode(&wave); err != nil {
		t.Fatal(err)
	}
	if wave.Buckets != 321 || len(wave.Peaks) != 321 || wave.Duration <= 0 {
		t.Fatalf("bad waveform: %+v", wave)
	}
}

func TestUploadRawWAVAndSensitivityRevision(t *testing.T) {
	ts := testServer(t)
	raw, err := demo.Generate()
	if err != nil {
		t.Fatal(err)
	}
	req, err := http.NewRequest(http.MethodPost, ts.URL+"/api/sessions?name=evidence.wav", bytes.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "audio/wav")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("upload failed: %d %s", resp.StatusCode, body)
	}
	var created createResponse
	if err := json.NewDecoder(resp.Body).Decode(&created); err != nil {
		t.Fatal(err)
	}

	sensResp := postJSON(t, ts.URL+"/api/sessions/"+created.ID+"/sensitivity", `{"sensitivity":0.10}`)
	defer sensResp.Body.Close()
	if sensResp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(sensResp.Body)
		t.Fatalf("sensitivity failed: %d %s", sensResp.StatusCode, body)
	}
	var revision struct {
		Stats    analysis.Stats    `json:"stats"`
		Revision analysis.Revision `json:"revision"`
	}
	if err := json.NewDecoder(sensResp.Body).Decode(&revision); err != nil {
		t.Fatal(err)
	}
	if revision.Revision.RecomputedIntervals == 0 || len(revision.Revision.Changes) == 0 {
		t.Fatalf("expected partial recomputation: %+v", revision.Revision)
	}
	if revision.Revision.Before.Intervals != revision.Revision.After.Intervals {
		t.Fatal("before/after interval counts are not aligned")
	}
	if revision.Stats.Flagged != revision.Revision.After.Flagged {
		t.Fatal("stats and revision disagree")
	}

	getResp, err := http.Get(ts.URL + "/api/sessions/" + created.ID)
	if err != nil {
		t.Fatal(err)
	}
	defer getResp.Body.Close()
	var current analysis.Analysis
	if err := json.NewDecoder(getResp.Body).Decode(&current); err != nil {
		t.Fatal(err)
	}
	if current.Stats.Flagged != revision.Stats.Flagged || current.Sensitivity != 0.10 {
		t.Fatalf("session did not retain revised state: %+v", current.Threshold)
	}

	noChange := postJSON(t, ts.URL+"/api/sessions/"+created.ID+"/sensitivity", `{"sensitivity":0.10}`)
	defer noChange.Body.Close()
	noChangeRaw, _ := io.ReadAll(noChange.Body)
	if !bytes.Contains(noChangeRaw, []byte(`"changes":[]`)) ||
		!bytes.Contains(noChangeRaw, []byte(`"recomputedRanges":[]`)) {
		t.Fatalf("empty revision arrays must encode as []: %s", noChangeRaw)
	}
}

func TestNoiseOnlyUploadReturnsEmptyRepeatArray(t *testing.T) {
	ts := testServer(t)
	const (
		sampleRate = 16000
		channels   = 1
		seconds    = 2
	)
	rng := rand.New(rand.NewSource(7))
	samples := make([]float64, sampleRate*seconds*channels)
	for i := range samples {
		samples[i] = 0.02 * rng.NormFloat64()
	}
	raw, err := audio.EncodePCM16(channels, sampleRate, samples)
	if err != nil {
		t.Fatal(err)
	}
	req, err := http.NewRequest(http.MethodPost, ts.URL+"/api/sessions?name=noise.wav", bytes.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "audio/wav")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("upload failed: %d %s", resp.StatusCode, body)
	}
	if bytes.Contains(body, []byte(`"repeats":null`)) || !bytes.Contains(body, []byte(`"repeats":[]`)) {
		t.Fatalf("repeats must be an empty array: %s", body)
	}
	var created createResponse
	if err := json.Unmarshal(body, &created); err != nil {
		t.Fatal(err)
	}
	for _, interval := range created.Analysis.Intervals {
		if interval.Flagged {
			t.Fatal("unexpected red interval in noise-only sample")
		}
	}
}

func TestSensitivityValidationAndNotFound(t *testing.T) {
	ts := testServer(t)
	created := createDemoSession(t, ts)
	resp := postJSON(t, ts.URL+"/api/sessions/"+created.ID+"/sensitivity", `{"sensitivity":1.5}`)
	resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("invalid sensitivity status %d", resp.StatusCode)
	}
	missing, err := http.Get(ts.URL + "/api/sessions/does-not-exist")
	if err != nil {
		t.Fatal(err)
	}
	missing.Body.Close()
	if missing.StatusCode != http.StatusNotFound {
		t.Fatalf("missing session status %d", missing.StatusCode)
	}
}

func TestIndexAndAssets(t *testing.T) {
	ts := testServer(t)
	resp, err := http.Get(ts.URL + "/")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK || !bytes.Contains(body, []byte("多证据复核")) {
		t.Fatalf("index not served: %d", resp.StatusCode)
	}
	if !strings.Contains(resp.Header.Get("Content-Security-Policy"), "script-src 'self'") {
		t.Fatal("missing content security policy")
	}
	asset, err := http.Get(ts.URL + "/static/app.js")
	if err != nil {
		t.Fatal(err)
	}
	asset.Body.Close()
	if asset.StatusCode != http.StatusOK {
		t.Fatalf("asset status %d", asset.StatusCode)
	}
}
