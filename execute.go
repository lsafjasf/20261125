package main

import (
	"fmt"
	"math"
	"path/filepath"
	"strings"
)

// OutputFile describes one PNG produced for a target node.
type OutputFile struct {
	Node   string `json:"node"`
	Path   string `json:"path"`
	Space  string `json:"space"`
	Width  int    `json:"width"`
	Height int    `json:"height"`
	Mean   RGB    `json:"mean"`
}

// FrameRun is the per-frame execution record.
type FrameRun struct {
	Frame   string       `json:"frame"`
	Outputs []OutputFile `json:"outputs"`
	Nodes   []string     `json:"executedNodes"`
}

// ExecutionReport aggregates all processed frames.
type ExecutionReport struct {
	SkippedUnused []string   `json:"skippedUnusedNodes"`
	Frames        []FrameRun `json:"frames"`
}

func cloneFrame(f *Frame) *Frame {
	pix := append([]RGB{}, f.Pix...)
	return &Frame{Name: f.Name, W: f.W, H: f.H, Space: f.Space, Pix: pix}
}

// RunFrames executes a valid plan over every supplied frame path.
func RunFrames(report *Report, framePaths []string, outDir string) (*ExecutionReport, error) {
	if !report.Valid {
		return nil, fmt.Errorf("refusing to execute invalid graph")
	}
	cfg := report.graph.Config
	planByNode := map[string]NodePlan{}
	for _, np := range report.ColorPlan {
		planByNode[np.Node] = np
	}
	used := report.graph.UsedSet(report.Targets)
	skipped := []string{}
	for _, u := range report.UnusedNodes {
		skipped = append(skipped, u.ID)
	}
	exec := &ExecutionReport{SkippedUnused: skipped, Frames: []FrameRun{}}
	for _, path := range framePaths {
		loaded, err := ReadPNG(path, "sRGB")
		if err != nil {
			return nil, fmt.Errorf("读取测试帧失败 %s: %w", path, err)
		}
		run := FrameRun{Frame: filepath.Base(path), Outputs: []OutputFile{}, Nodes: []string{}}
		buffers := map[string]*Frame{}
		for _, id := range report.TopologicalOrder {
			if !used[id] {
				continue
			}
			node := report.graph.NodeMap[id]
			var result *Frame
			if normalizeOp(node.Type) == "load" || normalizeOp(node.Type) == "source" {
				result = cloneFrame(loaded)
				if len(node.Outputs) > 0 && node.Outputs[0].Space != "" {
					result.Space = SpaceLabel(node.Outputs[0].Space)
				}
			} else {
				inputs, err := collectInputs(report, planByNode[id], buffers)
				if err != nil {
					return nil, err
				}
				result, err = applyOp(node, inputs)
				if err != nil {
					return nil, fmt.Errorf("节点 %q 执行失败: %w", id, err)
				}
			}
			if err := normalizeOutputSpace(node, cfg, result); err != nil {
				return nil, err
			}
			buffers[id] = result
			run.Nodes = append(run.Nodes, id)
		}
		base := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
		for _, target := range report.Targets {
			buf, ok := buffers[target]
			if !ok {
				continue
			}
			preview := cloneFrame(buf)
			if err := ConvertFrame(preview, "sRGB"); err != nil {
				return nil, err
			}
			outPath := filepath.Join(outDir, base+"__"+target+".png")
			if err := WritePNG(outPath, preview); err != nil {
				return nil, err
			}
			run.Outputs = append(run.Outputs, OutputFile{
				Node: target, Path: outPath, Space: buf.Space,
				Width: buf.W, Height: buf.H, Mean: meanRGB(buf),
			})
		}
		exec.Frames = append(exec.Frames, run)
	}
	return exec, nil
}

func collectInputs(report *Report, np NodePlan, buffers map[string]*Frame) ([]*Frame, error) {
	g := report.graph
	bindings := g.bindingsFor(np.Node)
	inputs := make([]*Frame, 0, len(bindings))
	for i, b := range bindings {
		buf, ok := buffers[b.Producer]
		if !ok {
			return nil, fmt.Errorf("节点 %q 的上游 %q 尚未产出", np.Node, b.Producer)
		}
		adopted := buf.Space
		if i < len(np.Inputs) && np.Inputs[i].AdoptedSpace != "" {
			adopted = np.Inputs[i].AdoptedSpace
		}
		wire := cloneFrame(buf)
		if adopted != "" {
			if err := ConvertFrame(wire, adopted); err != nil {
				return nil, fmt.Errorf("节点 %q 输入转换失败: %w", np.Node, err)
			}
		}
		inputs = append(inputs, wire)
	}
	return inputs, nil
}

func normalizeOutputSpace(node *Node, cfg *Config, f *Frame) error {
	target := ""
	if len(node.Outputs) > 0 && node.Outputs[0].Space != "" {
		target = node.Outputs[0].Space
	} else if to := paramString(node.Params, "to", "targetSpace", "outputSpace"); to != "" {
		target = to
	}
	if target == "" {
		return nil
	}
	if _, err := ResolveSpace(target); err != nil {
		return fmt.Errorf("节点 %q 输出色彩空间无效: %w", node.ID, err)
	}
	return ConvertFrame(f, target)
}

func meanRGB(f *Frame) RGB {
	var sum RGB
	for _, px := range f.Pix {
		sum[0] += px[0]
		sum[1] += px[1]
		sum[2] += px[2]
	}
	n := float64(len(f.Pix))
	if n == 0 {
		return sum
	}
	return RGB{sum[0] / n, sum[1] / n, sum[2] / n}
}

func applyOp(node *Node, inputs []*Frame) (*Frame, error) {
	op := normalizeOp(node.Type)
	switch op {
	case "load", "source":
		return nil, fmt.Errorf("源节点应由执行器直接注入帧")
	case "sink", "encode", "passthrough", "null":
		if len(inputs) == 0 {
			return nil, fmt.Errorf("%s 节点需要至少一个输入", op)
		}
		return cloneFrame(inputs[0]), nil
	case "convert":
		if len(inputs) == 0 {
			return nil, fmt.Errorf("convert 节点需要一个输入")
		}
		return cloneFrame(inputs[0]), nil
	case "gain":
		return applyGain(node, inputs)
	case "matrix":
		return applyMatrix(node, inputs)
	case "gamma":
		return applyGamma(node, inputs)
	case "clamp":
		return applyClamp(node, inputs)
	case "mix":
		return applyMix(node, inputs)
	default:
		return nil, fmt.Errorf("不支持的算子类型 %q（标准化为 %q）", node.Type, op)
	}
}

func requireUnary(node *Node, inputs []*Frame) (*Frame, error) {
	if len(inputs) != 1 {
		return nil, fmt.Errorf("%s 需要恰好 1 个输入，实际 %d", node.ID, len(inputs))
	}
	return cloneFrame(inputs[0]), nil
}

func applyGain(node *Node, inputs []*Frame) (*Frame, error) {
	f, err := requireUnary(node, inputs)
	if err != nil {
		return nil, err
	}
	scalar := paramFloat(node.Params, 1, "gain", "value", "scale")
	exposure := paramFloat(node.Params, 0, "exposure", "stops")
	if exposure != 0 {
		scalar *= math.Exp2(exposure)
	}
	tint := paramVec3(node.Params, RGB{1, 1, 1}, "rgb", "multiply", "tint")
	for i, px := range f.Pix {
		f.Pix[i] = RGB{px[0] * scalar * tint[0], px[1] * scalar * tint[1], px[2] * scalar * tint[2]}
	}
	return f, nil
}

func applyMatrix(node *Node, inputs []*Frame) (*Frame, error) {
	f, err := requireUnary(node, inputs)
	if err != nil {
		return nil, err
	}
	m, ok := paramMat3(node.Params, "matrix")
	if !ok {
		m = mat3{1, 0, 0, 0, 1, 0, 0, 0, 1}
	}
	offset := paramVec3(node.Params, RGB{0, 0, 0}, "offset", "add")
	for i, px := range f.Pix {
		v := m.mul([3]float64(px))
		f.Pix[i] = RGB{v[0] + offset[0], v[1] + offset[1], v[2] + offset[2]}
	}
	return f, nil
}

func applyGamma(node *Node, inputs []*Frame) (*Frame, error) {
	f, err := requireUnary(node, inputs)
	if err != nil {
		return nil, err
	}
	exponent := paramFloat(node.Params, 1, "exponent", "gamma", "power")
	for i, px := range f.Pix {
		f.Pix[i] = RGB{math.Pow(px[0], exponent), math.Pow(px[1], exponent), math.Pow(px[2], exponent)}
	}
	return f, nil
}

func applyClamp(node *Node, inputs []*Frame) (*Frame, error) {
	f, err := requireUnary(node, inputs)
	if err != nil {
		return nil, err
	}
	lo := paramFloat(node.Params, 0, "min", "lo")
	hi := paramFloat(node.Params, 1, "max", "hi")
	for i, px := range f.Pix {
		for c := 0; c < 3; c++ {
			v := px[c]
			if v < lo {
				v = lo
			}
			if v > hi {
				v = hi
			}
			px[c] = v
		}
		f.Pix[i] = px
	}
	return f, nil
}

func applyMix(node *Node, inputs []*Frame) (*Frame, error) {
	if len(inputs) == 0 {
		return nil, fmt.Errorf("mix 节点至少需要 1 个输入")
	}
	mode := paramString(node.Params, "mode", "blend")
	weights := paramWeights(node.Params, len(inputs))
	hasWeights := node.Params["weights"] != nil
	normalize := paramBool(node.Params, true, "normalize")
	switch mode {
	case "avg", "average", "mean":
		for i := range weights {
			weights[i] = 1 / float64(len(inputs))
		}
	case "weighted":
		if normalize {
			sum := 0.0
			for _, w := range weights {
				sum += w
			}
			if sum != 0 {
				for i := range weights {
					weights[i] /= sum
				}
			}
		}
	case "add", "sum":
		// keep raw weights
	default:
		if hasWeights && normalize {
			sum := 0.0
			for _, w := range weights {
				sum += w
			}
			if sum != 0 {
				for i := range weights {
					weights[i] /= sum
				}
			}
		} else if !hasWeights {
			for i := range weights {
				weights[i] = 1 / float64(len(inputs))
			}
		}
	}
	base := cloneFrame(inputs[0])
	for p := range base.Pix {
		var c RGB
		for i, in := range inputs {
			w := weights[i]
			px := in.Pix[p]
			c[0] += w * px[0]
			c[1] += w * px[1]
			c[2] += w * px[2]
		}
		base.Pix[p] = c
	}
	return base, nil
}
