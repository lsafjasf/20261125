package main

import (
	"crypto/sha256"
	"encoding/csv"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"time"
)

type setConfig struct {
	Label string `json:"label"`
	Dir   string `json:"dir"`
}

type auditConfig struct {
	ReferenceDir string     `json:"referenceDir"`
	SetA         setConfig  `json:"setA"`
	SetB         setConfig  `json:"setB"`
	TileSize     int        `json:"tileSize"`
	BlockSize    int        `json:"blockSize"`
	TopN         int        `json:"topN"`
	Thresholds   thresholds `json:"thresholds"`
}

type thresholds struct {
	NoiseFloorDelta float64 `json:"noiseFloorDelta"`
	BlockAlignMin   float64 `json:"blockAlignMin"`
	BandRateMin     float64 `json:"bandRateMin"`
	ChromaDriftMin  float64 `json:"chromaDriftMin"`
	DarkLumaMax     int     `json:"darkLumaMax"`
	DarkSmoothMin   float64 `json:"darkSmoothMin"`
}

type frameComparison struct {
	Index       int            `json:"index"`
	Frame       string         `json:"frame"`
	PSNRA       float64        `json:"psnrA"`
	PSNRB       float64        `json:"psnrB"`
	PSNRDeltaAB float64        `json:"psnrDeltaAB"`
	BandStepsA  int            `json:"bandStepsA"`
	BandStepsB  int            `json:"bandStepsB"`
	BlockScoreA float64        `json:"blockScoreA"`
	BlockScoreB float64        `json:"blockScoreB"`
	RMSECbA     float64        `json:"rmseCbA"`
	RMSECrA     float64        `json:"rmseCrA"`
	RMSECbB     float64        `json:"rmseCbB"`
	RMSECrB     float64        `json:"rmseCrB"`
	TilesA      map[string]int `json:"tilesA"`
	TilesB      map[string]int `json:"tilesB"`
}

type inspectionItem struct {
	Rank        int     `json:"rank"`
	Set         string  `json:"set"`
	Frame       string  `json:"frame"`
	FrameIndex  int     `json:"frameIndex"`
	TileCol     int     `json:"tileCol"`
	TileRow     int     `json:"tileRow"`
	X0          int     `json:"x0"`
	Y0          int     `json:"y0"`
	X1          int     `json:"x1"`
	Y1          int     `json:"y1"`
	Class       string  `json:"class"`
	Severity    float64 `json:"severity"`
	MSE         float64 `json:"mse"`
	BlockScore  float64 `json:"blockScore"`
	BandScore   float64 `json:"bandScore"`
	ChromaScore float64 `json:"chromaScore"`
	Suggestion  string  `json:"suggestion"`
}

type setSummary struct {
	Label           string  `json:"label"`
	Frames          int     `json:"frames"`
	MeanPSNRY       float64 `json:"meanPsnrY"`
	MeanRMSECb      float64 `json:"meanRmseCb"`
	MeanRMSECr      float64 `json:"meanRmseCr"`
	MeanBlockScore  float64 `json:"meanBlockScore"`
	TotalBandSteps  int     `json:"totalBandSteps"`
	TilesCoding     int     `json:"tilesCoding"`
	TilesConversion int     `json:"tilesConversion"`
	TilesMixed      int     `json:"tilesMixed"`
}

type report struct {
	RunID      string            `json:"runId"`
	CreatedUTC string            `json:"createdUtc"`
	Config     auditConfig       `json:"config"`
	SummaryA   setSummary        `json:"summaryA"`
	SummaryB   setSummary        `json:"summaryB"`
	Frames     []frameComparison `json:"frames"`
	Inspection []inspectionItem  `json:"inspection"`
	NextCheck  string            `json:"nextCheck"`
}

type fileFingerprint struct {
	Path   string `json:"path"`
	Size   int64  `json:"size"`
	SHA256 string `json:"sha256"`
}

type manifest struct {
	RunID      string            `json:"runId"`
	CreatedUTC string            `json:"createdUtc"`
	GoVersion  string            `json:"goVersion"`
	Config     auditConfig       `json:"config"`
	Inputs     []fileFingerprint `json:"inputs"`
	FrameOrder []string          `json:"frameOrder"`
	Outputs    []string          `json:"outputs"`
}

func buildConfig(refDir string, a, b setConfig, topN int) auditConfig {
	if topN <= 0 {
		topN = 10
	}
	return auditConfig{
		ReferenceDir: refDir, SetA: a, SetB: b,
		TileSize: tileSize, BlockSize: blockSize, TopN: topN,
		Thresholds: thresholds{
			NoiseFloorDelta: noiseFloorDelta, BlockAlignMin: blockAlignMin,
			BandRateMin: bandRateMin, ChromaDriftMin: chromaDriftMin,
			DarkLumaMax: darkLumaMax, DarkSmoothMin: darkSmoothMin,
		},
	}
}

// severity is a deterministic weighted score. Conversion signals (band steps,
// chroma drift in dark smooth areas) weigh heavily because mean PSNR barely
// moves when they appear.
func severity(t tileStats) float64 {
	return t.MSE/25.0 +
		t.BandScore*500.0 +
		math.Max(0, t.BlockScore-1.0)*4.0 +
		t.ChromaScore*0.5 +
		t.DarkSmoothFrac*2.0
}

func suggestionFor(class string) string {
	switch class {
	case classConversion:
		return "检查暗部色带与色度漂移：核对色彩矩阵(BT.709/601)、全/有限范围、量化位深与抖动"
	case classCoding:
		return "检查块边界与量化：核对 QP、码率分配、去块效应/环路滤波"
	default:
		return "同时检查编码块边界与色彩转换链路"
	}
}

func rankInspection(perSet map[string][]*frameStats, cfg auditConfig) []inspectionItem {
	type scored struct {
		t  tileStats
		fi int
	}
	var all []scored
	for setName, frames := range perSet {
		for fi, fs := range frames {
			for _, t := range fs.Tiles {
				if t.Class == classNone || t.MeanAbsDelta < cfg.Thresholds.NoiseFloorDelta {
					continue
				}
				all = append(all, scored{t, fi})
			}
		}
		_ = setName
	}
	sort.SliceStable(all, func(i, j int) bool {
		si, sj := severity(all[i].t), severity(all[j].t)
		if si != sj {
			return si > sj
		}
		if all[i].t.Set != all[j].t.Set {
			return all[i].t.Set < all[j].t.Set
		}
		if all[i].fi != all[j].fi {
			return all[i].fi < all[j].fi
		}
		if all[i].t.Row != all[j].t.Row {
			return all[i].t.Row < all[j].t.Row
		}
		return all[i].t.Col < all[j].t.Col
	})

	limit := cfg.TopN
	if len(all) < limit {
		limit = len(all)
	}
	items := make([]inspectionItem, 0, limit)
	for k := 0; k < limit; k++ {
		s := all[k]
		t := s.t
		items = append(items, inspectionItem{
			Rank: k + 1, Set: t.Set, Frame: t.Frame, FrameIndex: s.fi,
			TileCol: t.Col, TileRow: t.Row,
			X0: t.X0, Y0: t.Y0, X1: t.X1, Y1: t.Y1,
			Class: t.Class, Severity: severity(t),
			MSE: t.MSE, BlockScore: t.BlockScore,
			BandScore: t.BandScore, ChromaScore: t.ChromaScore,
			Suggestion: suggestionFor(t.Class),
		})
	}
	return items
}

func buildSummary(label string, frames []*frameStats) setSummary {
	s := setSummary{Label: label, Frames: len(frames)}
	var pSum, cbSum, crSum, blkSum float64
	for _, fs := range frames {
		pSum += fs.PSNRY
		cbSum += fs.RMSECb
		crSum += fs.RMSECr
		blkSum += fs.BlockScore
		s.TotalBandSteps += fs.BandSteps
		s.TilesCoding += fs.TileCounts[classCoding]
		s.TilesConversion += fs.TileCounts[classConversion]
		s.TilesMixed += fs.TileCounts[classMixed]
	}
	n := math.Max(1, float64(len(frames)))
	s.MeanPSNRY = pSum / n
	s.MeanRMSECb = cbSum / n
	s.MeanRMSECr = crSum / n
	s.MeanBlockScore = blkSum / n
	return s
}

func buildNextCheck(items []inspectionItem) string {
	if len(items) == 0 {
		return "未发现超过噪声门限的伪影 tile，下一次可提高 -top 或复查最暗场景帧"
	}
	top := items[0]
	return fmt.Sprintf(
		"下一次应检查：%s 参数集 第%d帧 %s tile(col=%d,row=%d) 区域 x[%d:%d] y[%d:%d]，判定=%s。%s",
		top.Set, top.FrameIndex+1, top.Frame, top.TileCol, top.TileRow,
		top.X0, top.X1, top.Y0, top.Y1, top.Class, top.Suggestion,
	)
}

func fingerprint(path string) (fileFingerprint, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return fileFingerprint{}, err
	}
	sum := sha256.Sum256(data)
	return fileFingerprint{
		Path: path, Size: int64(len(data)),
		SHA256: hex.EncodeToString(sum[:]),
	}, nil
}

func writeOutputs(outDir string, rep *report, man *manifest) error {
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return err
	}
	repJSON, err := json.MarshalIndent(rep, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(outDir, "report.json"), append(repJSON, '\n'), 0o644); err != nil {
		return err
	}
	if err := writeInspectionCSV(filepath.Join(outDir, "inspection.csv"), rep.Inspection); err != nil {
		return err
	}
	manJSON, err := json.MarshalIndent(man, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(outDir, "manifest.json"), append(manJSON, '\n'), 0o644); err != nil {
		return err
	}
	man.Outputs = []string{"report.json", "inspection.csv", "manifest.json"}
	return nil
}

func writeInspectionCSV(path string, items []inspectionItem) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	w := csv.NewWriter(f)
	header := []string{"rank", "set", "frame", "frameIndex", "tileCol", "tileRow",
		"x0", "y0", "x1", "y1", "class", "severity", "mse",
		"blockScore", "bandScore", "chromaScore", "suggestion"}
	if err := w.Write(header); err != nil {
		return err
	}
	for _, it := range items {
		row := []string{
			fmt.Sprintf("%d", it.Rank), it.Set, it.Frame, fmt.Sprintf("%d", it.FrameIndex),
			fmt.Sprintf("%d", it.TileCol), fmt.Sprintf("%d", it.TileRow),
			fmt.Sprintf("%d", it.X0), fmt.Sprintf("%d", it.Y0),
			fmt.Sprintf("%d", it.X1), fmt.Sprintf("%d", it.Y1),
			it.Class, fmt.Sprintf("%.4f", it.Severity), fmt.Sprintf("%.4f", it.MSE),
			fmt.Sprintf("%.4f", it.BlockScore), fmt.Sprintf("%.6f", it.BandScore),
			fmt.Sprintf("%.4f", it.ChromaScore), it.Suggestion,
		}
		if err := w.Write(row); err != nil {
			return err
		}
	}
	w.Flush()
	return w.Error()
}

func newRunID(now time.Time) string {
	return "run_" + now.UTC().Format("20060102T150405Z")
}

func buildManifest(cfg auditConfig, runID string, now time.Time, inputPaths, frameOrder []string) (*manifest, error) {
	fps := make([]fileFingerprint, 0, len(inputPaths))
	for _, p := range inputPaths {
		fp, err := fingerprint(p)
		if err != nil {
			return nil, err
		}
		fps = append(fps, fp)
	}
	return &manifest{
		RunID: runID, CreatedUTC: now.UTC().Format(time.RFC3339),
		GoVersion: runtime.Version(), Config: cfg,
		Inputs: fps, FrameOrder: frameOrder,
	}, nil
}
