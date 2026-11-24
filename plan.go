package main

// InputColorPlan explains the color space actually adopted for one input.
type InputColorPlan struct {
	InputName     string `json:"inputName,omitempty"`
	Ref           string `json:"ref"`
	FromNode      string `json:"fromNode,omitempty"`
	Handle        string `json:"handle,omitempty"`
	IncomingSpace string `json:"incomingSpace,omitempty"`
	AssumeSpace   string `json:"assumeSpace,omitempty"`
	AdoptedSpace  string `json:"adoptedSpace"`
	ExpectSpace   string `json:"expectSpace,omitempty"`
	Resolution    string `json:"resolution"`
	Conversion    string `json:"conversion,omitempty"`
	AutoConverted bool   `json:"autoConverted"`
	Status        string `json:"status"`
	Message       string `json:"message,omitempty"`
}

// NodePlan is the per-node "effective input color space" statement.
type NodePlan struct {
	Node         string            `json:"node"`
	Type         string            `json:"type"`
	WorkingSpace string            `json:"workingSpace,omitempty"`
	Inputs       []InputColorPlan  `json:"inputs"`
	Outputs      []OutputSpacePlan `json:"outputs,omitempty"`
}

type OutputSpacePlan struct {
	Name  string `json:"name"`
	Space string `json:"space,omitempty"`
}

// Report is the full preflight analysis rendered as text or JSON.
type Report struct {
	Name             string            `json:"name,omitempty"`
	Valid            bool              `json:"valid"`
	DefaultSpace     string            `json:"defaultColorSpace"`
	AutoConvert      bool              `json:"autoConvert"`
	TopologicalOrder []string          `json:"topologicalOrder"`
	Forks            []Fork            `json:"forks"`
	Merges           []Merge           `json:"merges"`
	Cycles           []CycleInfo       `json:"cycles"`
	DuplicateOutputs []DuplicateOutput `json:"duplicateOutputs"`
	UnusedNodes      []UnusedNode      `json:"unusedNodes"`
	UnusedOutputs    []string          `json:"unusedOutputs"`
	Targets          []string          `json:"targets"`
	ColorPlan        []NodePlan        `json:"colorPlan"`
	Diagnostics      []Diagnostic      `json:"diagnostics"`

	graph     *Graph    `json:"-"`
	structure Structure `json:"-"`
}

// BuildReport performs structural analysis and color-space resolution.
func BuildReport(cfg *Config) *Report {
	if cfg.DefaultSpace == "" {
		cfg.DefaultSpace = "sRGB"
	}
	g := BuildGraph(cfg)
	targets := g.ResolveTargets()
	order, topoErr := g.TopoSort()
	st := g.Classify(targets, order, topoErr)

	for _, cyc := range st.Cycles {
		g.errorf("cycle_detected", "", "检测到真正的循环依赖: %s", joinPath(cyc.Path))
	}

	r := &Report{
		Name:             cfg.Name,
		DefaultSpace:     SpaceLabel(cfg.DefaultSpace),
		AutoConvert:      cfg.AutoConvertEnabled(),
		TopologicalOrder: st.TopologicalOrder,
		Forks:            orEmptyForks(st.Forks),
		Merges:           orEmptyMerges(st.Merges),
		Cycles:           orEmptyCycles(st.Cycles),
		DuplicateOutputs: orEmptyDup(g.Duplicates),
		UnusedNodes:      orEmptyUnused(st.UnusedNodes),
		UnusedOutputs:    orEmptyStrings(st.UnusedOutputs),
		Targets:          orEmptyStrings(targets),
		Diagnostics:      []Diagnostic{},
		graph:            g,
		structure:        st,
	}

	r.ColorPlan = g.buildColorPlan()
	r.DuplicateOutputs = orEmptyDup(g.Duplicates)
	r.Diagnostics = append([]Diagnostic{}, g.Diags...)
	r.Valid = !g.HasErrors() && !r.hasColorErrors()
	return r
}

func (r *Report) hasColorErrors() bool {
	for _, d := range r.Diagnostics {
		if d.Severity == "error" && (d.Code == "invalid_color_space" || d.Code == "color_space_mismatch") {
			return true
		}
	}
	return false
}

func (g *Graph) buildColorPlan() []NodePlan {
	var plans []NodePlan
	order := g.colorPlanOrder()
	for _, id := range order {
		n := g.NodeMap[id]
		np := NodePlan{Node: id, Type: n.Type, Inputs: []InputColorPlan{}, Outputs: []OutputSpacePlan{}}
		for _, out := range n.Outputs {
			np.Outputs = append(np.Outputs, OutputSpacePlan{Name: out.Name, Space: SpaceLabel(out.Space)})
			if out.Space != "" {
				if _, err := ResolveSpace(out.Space); err != nil {
					g.errorf("invalid_color_space", id, "节点 %q 输出 %q 声明了无法识别的色彩空间 %q", id, out.Name, out.Space)
				}
			}
		}
		nodeWorking := paramString(n.Params, "workingSpace", "inputSpace", "inputColorSpace")
		if nodeWorking != "" {
			if _, err := ResolveSpace(nodeWorking); err != nil {
				g.errorf("invalid_color_space", id, "节点 %q 的 workingSpace 无法识别: %v", id, err)
			}
			np.WorkingSpace = SpaceLabel(nodeWorking)
		}
		bindings := g.bindingsFor(id)
		mergeLike := len(bindings) >= 2 && isMergeOp(n.Type)
		if nodeWorking == "" && mergeLike && len(bindings) > 0 {
			nodeWorking, _ = bindings[0].incomingSpace(g.Config.DefaultSpace)
			np.WorkingSpace = SpaceLabel(nodeWorking)
		}
		for _, b := range bindings {
			ip := g.resolveInputColor(b, nodeWorking)
			np.Inputs = append(np.Inputs, ip)
			if ip.Status == "mismatch" {
				g.errorf("color_space_mismatch", id, "节点 %q 输入 %q 色彩空间为 %s，但操作要求 %s 且 autoConvert=false",
					id, ip.Ref, ip.IncomingSpace, ip.ExpectSpace)
			}
			if ip.Status == "invalid-space" {
				g.errorf("invalid_color_space", id, "节点 %q 输入 %q: %s", id, ip.Ref, ip.Message)
			}
		}
		plans = append(plans, np)
	}
	return plans
}

type boundInput struct {
	*Binding
}

func (g *Graph) bindingsFor(nodeID string) []*Binding {
	var out []*Binding
	for _, b := range g.Bindings {
		if b.Consumer == nodeID {
			out = append(out, b)
		}
	}
	return out
}

func (b *Binding) incomingSpace(defaultSpace string) (space, resolution string) {
	switch {
	case b.WireSpace != "":
		return b.WireSpace, "output-declared"
	case b.AssumeSpace != "":
		return b.AssumeSpace, "input-assumption"
	case defaultSpace != "":
		return defaultSpace, "config-default"
	default:
		return "sRGB", "built-in-default"
	}
}

func (g *Graph) resolveInputColor(b *Binding, nodeWorking string) InputColorPlan {
	ip := InputColorPlan{
		InputName:   b.InputName,
		Ref:         b.Ref,
		FromNode:    b.Producer,
		Handle:      b.Handle,
		AssumeSpace: b.AssumeSpace,
		ExpectSpace: b.ExpectSpace,
		Status:      "ok",
	}
	incomingRaw, resolution := b.incomingSpace(g.Config.DefaultSpace)
	incoming, errIn := ResolveSpace(incomingRaw)
	if errIn != nil {
		ip.Status = "invalid-space"
		ip.AdoptedSpace = incomingRaw
		ip.Resolution = resolution
		ip.Message = errIn.Error()
		return ip
	}
	ip.IncomingSpace = incoming.Label
	ip.Resolution = resolution

	targetRaw := firstNonEmpty(b.ExpectSpace, nodeWorking, incoming.Label)
	target, errT := ResolveSpace(targetRaw)
	if errT != nil {
		ip.Status = "invalid-space"
		ip.AdoptedSpace = targetRaw
		ip.Message = errT.Error()
		return ip
	}
	ip.ExpectSpace = ""
	if b.ExpectSpace != "" {
		ip.ExpectSpace = target.Label
	}
	if nodeWorking != "" && ip.ExpectSpace == "" {
		ip.ExpectSpace = target.Label
	}

	if target.ID != incoming.ID {
		if g.Config.AutoConvertEnabled() {
			ip.AutoConverted = true
			ip.Conversion = incoming.Label + " -> " + target.Label
			ip.AdoptedSpace = target.Label
		} else {
			ip.Status = "mismatch"
			ip.AdoptedSpace = incoming.Label
			ip.ExpectSpace = target.Label
			ip.Message = "autoConvert=false，不能隐式转换"
		}
	} else {
		ip.AdoptedSpace = incoming.Label
	}
	return ip
}

// colorPlanOrder prefers a topological order; on cyclic graphs it falls back
// to configuration order so every node still gets an explanation.
func (g *Graph) colorPlanOrder() []string {
	if order, err := g.TopoSort(); err == nil {
		return order
	}
	return append([]string{}, g.IDs...)
}

func isMergeOp(t string) bool {
	switch normalizeOp(t) {
	case "mix", "merge", "blend", "composite", "average", "add", "sum", "over":
		return true
	}
	return false
}

func orEmptyForks(v []Fork) []Fork {
	if v == nil {
		return []Fork{}
	}
	return v
}
func orEmptyMerges(v []Merge) []Merge {
	if v == nil {
		return []Merge{}
	}
	return v
}
func orEmptyDup(v []DuplicateOutput) []DuplicateOutput {
	if v == nil {
		return []DuplicateOutput{}
	}
	return v
}
func orEmptyUnused(v []UnusedNode) []UnusedNode {
	if v == nil {
		return []UnusedNode{}
	}
	return v
}
func orEmptyStrings(v []string) []string {
	if v == nil {
		return []string{}
	}
	return v
}
func orEmptyCycles(v []CycleInfo) []CycleInfo {
	if v == nil {
		return []CycleInfo{}
	}
	return v
}
