package main

import (
	"fmt"
	"sort"
)

// Diagnostic is a structured configuration problem.
type Diagnostic struct {
	Severity string `json:"severity"`
	Code     string `json:"code"`
	Node     string `json:"node,omitempty"`
	Message  string `json:"message"`
}

// DuplicateOutput marks one handle declared by multiple nodes.
type DuplicateOutput struct {
	Handle     string   `json:"handle"`
	DeclaredBy []string `json:"declaredBy"`
	Spaces     []string `json:"spaces,omitempty"`
}

// Binding is a resolved input socket wired to a producer output.
type Binding struct {
	Consumer     string `json:"consumer"`
	InputIndex   int    `json:"inputIndex"`
	InputName    string `json:"inputName,omitempty"`
	Ref          string `json:"ref"`
	Producer     string `json:"producer,omitempty"`
	Handle       string `json:"handle,omitempty"`
	OutputSocket string `json:"outputSocket,omitempty"`
	WireSpace    string `json:"wireSpace,omitempty"`
	AssumeSpace  string `json:"assumeSpace,omitempty"`
	ExpectSpace  string `json:"expectSpace,omitempty"`
	ExplicitEdge bool   `json:"explicitEdge,omitempty"`
	Resolved     bool   `json:"resolved"`
}

type handleDecl struct {
	node  *Node
	spec  OutputSpec
	index int
}

// Graph is the resolved dependency graph plus all structural findings.
type Graph struct {
	Config     *Config
	NodeMap    map[string]*Node
	IDs        []string
	Handles    map[string][]handleDecl
	Bindings   []*Binding
	Edges      map[string]map[string]bool
	Reverse    map[string]map[string]bool
	Diags      []Diagnostic
	Duplicates []DuplicateOutput
}

func (g *Graph) errorf(code, node, format string, args ...any) {
	g.Diags = append(g.Diags, Diagnostic{
		Severity: "error", Code: code, Node: node,
		Message: fmt.Sprintf(format, args...),
	})
}

func (g *Graph) warnf(code, node, format string, args ...any) {
	g.Diags = append(g.Diags, Diagnostic{
		Severity: "warning", Code: code, Node: node,
		Message: fmt.Sprintf(format, args...),
	})
}

func (g *Graph) addEdge(from, to string) {
	if from == "" || to == "" {
		return
	}
	if g.Edges[from] == nil {
		g.Edges[from] = map[string]bool{}
	}
	if g.Reverse[to] == nil {
		g.Reverse[to] = map[string]bool{}
	}
	g.Edges[from][to] = true
	g.Reverse[to][from] = true
}

func sortedKeys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// BuildGraph parses configuration into an indexed graph and records
// duplicate outputs and unresolved references.
func BuildGraph(cfg *Config) *Graph {
	g := &Graph{
		Config:  cfg,
		NodeMap: map[string]*Node{},
		Handles: map[string][]handleDecl{},
		Edges:   map[string]map[string]bool{},
		Reverse: map[string]map[string]bool{},
	}
	for i, n := range cfg.Nodes {
		n.Index = i
		if n.ID == "" {
			n.ID = fmt.Sprintf("__unnamed_%d", i)
			g.errorf("missing_node_id", n.ID, "第 %d 个节点缺少 id/name", i+1)
		}
		if _, dup := g.NodeMap[n.ID]; dup {
			g.errorf("duplicate_node_id", n.ID, "节点 id %q 重复声明", n.ID)
			continue
		}
		g.NodeMap[n.ID] = n
		g.IDs = append(g.IDs, n.ID)
		for j, out := range n.Outputs {
			if out.Name == "" {
				out.Name = fmt.Sprintf("%s.out%d", n.ID, j)
				n.Outputs[j].Name = out.Name
			}
			g.Handles[out.Name] = append(g.Handles[out.Name], handleDecl{node: n, spec: out, index: j})
		}
	}

	for handle, decls := range g.Handles {
		if len(decls) > 1 {
			d := DuplicateOutput{Handle: handle}
			for _, dc := range decls {
				d.DeclaredBy = append(d.DeclaredBy, dc.node.ID)
				d.Spaces = append(d.Spaces, dc.spec.Space)
			}
			g.Duplicates = append(g.Duplicates, d)
			g.errorf("duplicate_output", "", "输出句柄 %q 被节点 %v 重复声明", handle, d.DeclaredBy)
		}
	}
	sort.Slice(g.Duplicates, func(i, j int) bool { return g.Duplicates[i].Handle < g.Duplicates[j].Handle })

	for _, id := range g.IDs {
		n := g.NodeMap[id]
		for idx, in := range n.Inputs {
			b := &Binding{
				Consumer:    id,
				InputIndex:  idx,
				InputName:   in.Name,
				Ref:         in.Ref,
				AssumeSpace: in.Space,
				ExpectSpace: in.Expect,
			}
			g.resolveBinding(b, in.Socket)
			g.Bindings = append(g.Bindings, b)
			if b.Resolved {
				g.addEdge(b.Producer, id)
			} else if in.Ref != "" {
				g.errorf("unknown_reference", id, "节点 %q 的输入 %q 找不到对应输出句柄或节点", id, in.Ref)
			}
		}
		if len(n.Inputs) == 0 && !isSourceLike(n.Type) {
			g.warnf("inputless_node", id, "节点 %q 类型 %q 没有任何输入", id, n.Type)
		}
	}

	seenPair := map[string]bool{}
	for _, e := range cfg.Edges {
		from, fromHandle, okFrom := g.resolveEndpoint(e.From)
		to, _, okTo := g.resolveEndpoint(e.To)
		if !okFrom || !okTo {
			g.errorf("bad_edge", "", "显式边 %q -> %q 存在无法解析的端点", e.From, e.To)
			continue
		}
		key := from + "\x00" + to
		if seenPair[key] {
			continue
		}
		seenPair[key] = true
		g.addEdge(from, to)
		if !g.hasBinding(from, to) {
			space := ""
			socket := ""
			if decls := g.Handles[fromHandle]; len(decls) == 1 {
				space = decls[0].spec.Space
				socket = fromHandle
			} else if out := g.singleOutput(from); out != nil {
				space = out.Space
				socket = out.Name
			}
			b := &Binding{
				Consumer: to, Ref: e.From, Producer: from, Handle: socket,
				OutputSocket: socket, WireSpace: space, Resolved: true, ExplicitEdge: true,
			}
			g.Bindings = append(g.Bindings, b)
		}
	}
	return g
}

func (g *Graph) hasBinding(from, to string) bool {
	for _, b := range g.Bindings {
		if b.Producer == from && b.Consumer == to {
			return true
		}
	}
	return false
}

func (g *Graph) singleOutput(nodeID string) *OutputSpec {
	n, ok := g.NodeMap[nodeID]
	if !ok || len(n.Outputs) == 0 {
		return nil
	}
	return &n.Outputs[0]
}

// resolveEndpoint maps a handle or node id to a node id.
func (g *Graph) resolveEndpoint(ref string) (nodeID, handle string, ok bool) {
	if ref == "" {
		return "", "", false
	}
	if decls, exists := g.Handles[ref]; exists {
		if len(decls) >= 1 {
			return decls[0].node.ID, ref, true
		}
	}
	if n, exists := g.NodeMap[ref]; exists {
		h := ""
		if len(n.Outputs) == 1 {
			h = n.Outputs[0].Name
		}
		return ref, h, true
	}
	return "", "", false
}

func (g *Graph) resolveBinding(b *Binding, socket string) {
	ref := b.Ref
	if ref == "" {
		return
	}
	if decls, exists := g.Handles[ref]; exists {
		// Even with duplicates the first declaration is used for structure;
		// duplicate_output is already reported as an error.
		dc := decls[0]
		b.Producer = dc.node.ID
		b.Handle = dc.spec.Name
		b.OutputSocket = dc.spec.Name
		b.WireSpace = dc.spec.Space
		b.Resolved = true
		return
	}
	if n, exists := g.NodeMap[ref]; exists {
		b.Producer = ref
		b.Resolved = true
		out := g.pickOutput(n, socket)
		if out != nil {
			b.Handle = out.Name
			b.OutputSocket = out.Name
			b.WireSpace = out.Space
		} else if socket != "" {
			g.warnf("unknown_socket", ref, "节点 %q 没有名为 %q 的输出插口", ref, socket)
		}
		if socket == "" && len(n.Outputs) > 1 {
			g.warnf("ambiguous_socket", ref, "引用节点 %q 但未指定输出插口，默认采用第一个输出", ref)
		}
	}
}

func (g *Graph) pickOutput(n *Node, socket string) *OutputSpec {
	if socket != "" {
		for i := range n.Outputs {
			if n.Outputs[i].Name == socket {
				return &n.Outputs[i]
			}
		}
		return nil
	}
	if len(n.Outputs) > 0 {
		return &n.Outputs[0]
	}
	return nil
}

func isSourceLike(t string) bool {
	switch normalizeOp(t) {
	case "load", "source", "input", "reader", "decode", "generator", "colorbars", "blank":
		return true
	}
	return false
}

func (g *Graph) HasErrors() bool {
	for _, d := range g.Diags {
		if d.Severity == "error" {
			return true
		}
	}
	return len(g.Duplicates) > 0
}
