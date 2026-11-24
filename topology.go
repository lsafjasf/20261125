package main

import "sort"

// Fork describes one output stream legally fanning out to >=2 consumers.
type Fork struct {
	Handle    string   `json:"handle,omitempty"`
	Source    string   `json:"source"`
	Consumers []string `json:"consumers"`
	Legal     bool     `json:"legal"`
}

// Merge describes one node legally converging >=2 predecessor branches.
type Merge struct {
	Node      string   `json:"node"`
	Producers []string `json:"producers"`
	Legal     bool     `json:"legal"`
}

// CycleInfo is a true dependency cycle. Path repeats the first node at the end.
type CycleInfo struct {
	Nodes []string `json:"nodes"`
	Path  []string `json:"path"`
}

// UnusedNode explains why a node is considered unused.
type UnusedNode struct {
	ID     string `json:"id"`
	Type   string `json:"type"`
	Reason string `json:"reason"`
}

type Structure struct {
	TopologicalOrder []string     `json:"topologicalOrder"`
	Forks            []Fork       `json:"forks"`
	Merges           []Merge      `json:"merges"`
	Cycles           []CycleInfo  `json:"cycles"`
	UnusedNodes      []UnusedNode `json:"unusedNodes"`
	UnusedOutputs    []string     `json:"unusedOutputs"`
	Targets          []string     `json:"targets"`
}

// CycleError is returned by TopoSort when the graph contains cycles.
type CycleError struct {
	Cycles []CycleInfo
}

func (e *CycleError) Error() string {
	if len(e.Cycles) == 0 {
		return "graph contains a dependency cycle"
	}
	return "graph contains a dependency cycle: " + joinPath(e.Cycles[0].Path)
}

func joinPath(p []string) string {
	out := ""
	for i, n := range p {
		if i > 0 {
			out += " -> "
		}
		out += n
	}
	return out
}

// indegrees computes distinct-predecessor counts.
func (g *Graph) indegrees() map[string]int {
	indeg := map[string]int{}
	for _, id := range g.IDs {
		indeg[id] = len(g.Reverse[id])
	}
	return indeg
}

// TopoSort runs Kahn's algorithm with smallest-id tie breaking for
// deterministic output. It returns the full order for DAGs, or a CycleError
// carrying the detected cycles when the graph is cyclic.
func (g *Graph) TopoSort() ([]string, error) {
	indeg := g.indegrees()
	ready := []string{}
	for _, id := range g.IDs {
		if indeg[id] == 0 {
			ready = append(ready, id)
		}
	}
	sort.Strings(ready)
	order := []string{}
	for len(ready) > 0 {
		id := ready[0]
		ready = ready[1:]
		order = append(order, id)
		for _, next := range sortedKeys(g.Edges[id]) {
			indeg[next]--
			if indeg[next] == 0 {
				ready = append(ready, next)
			}
		}
		sort.Strings(ready)
	}
	if len(order) != len(g.IDs) {
		cycles := g.FindCycles()
		return order, &CycleError{Cycles: cycles}
	}
	return order, nil
}

// FindCycles returns representative paths for every cyclic SCC.
func (g *Graph) FindCycles() []CycleInfo {
	sccs := g.tarjan()
	var cycles []CycleInfo
	for _, members := range sccs {
		set := map[string]bool{}
		for _, m := range members {
			set[m] = true
		}
		selfLoop := false
		if len(members) == 1 {
			id := members[0]
			selfLoop = g.Edges[id][id]
			if !selfLoop {
				continue
			}
		}
		path := g.representativeCycle(set)
		if selfLoop && (len(path) == 0) {
			path = []string{members[0], members[0]}
		}
		cycles = append(cycles, CycleInfo{Nodes: append([]string{}, members...), Path: path})
	}
	sort.Slice(cycles, func(i, j int) bool {
		if len(cycles[i].Path) == 0 || len(cycles[j].Path) == 0 {
			return cycles[i].Nodes[0] < cycles[j].Nodes[0]
		}
		return cycles[i].Path[0] < cycles[j].Path[0]
	})
	return cycles
}

// tarjan returns strongly connected components, node ids sorted inside each.
func (g *Graph) tarjan() [][]string {
	index := 0
	indices := map[string]int{}
	low := map[string]int{}
	onStack := map[string]bool{}
	stack := []string{}
	var sccs [][]string

	var visit func(v string)
	visit = func(v string) {
		indices[v] = index
		low[v] = index
		index++
		stack = append(stack, v)
		onStack[v] = true
		for _, w := range sortedKeys(g.Edges[v]) {
			if _, seen := indices[w]; !seen {
				visit(w)
				if low[w] < low[v] {
					low[v] = low[w]
				}
			} else if onStack[w] && indices[w] < low[v] {
				low[v] = indices[w]
			}
		}
		if low[v] == indices[v] {
			var scc []string
			for {
				w := stack[len(stack)-1]
				stack = stack[:len(stack)-1]
				onStack[w] = false
				scc = append(scc, w)
				if w == v {
					break
				}
			}
			sort.Strings(scc)
			sccs = append(sccs, scc)
		}
	}
	for _, id := range g.IDs {
		if _, seen := indices[id]; !seen {
			visit(id)
		}
	}
	return sccs
}

// representativeCycle finds one simple loop inside a cyclic SCC.
func (g *Graph) representativeCycle(set map[string]bool) []string {
	start := ""
	bestIndex := int(^uint(0) >> 1)
	for id := range set {
		if n := g.NodeMap[id]; n != nil && n.Index < bestIndex {
			bestIndex = n.Index
			start = id
		}
	}
	if start == "" {
		for id := range set {
			start = id
			break
		}
	}
	path := []string{start}
	localSeen := map[string]bool{start: true}
	var dfs func(cur string) []string
	dfs = func(cur string) []string {
		for _, next := range sortedKeys(g.Edges[cur]) {
			if !set[next] {
				continue
			}
			if next == start && len(path) >= 1 {
				return append(append([]string{}, path...), start)
			}
			if localSeen[next] {
				continue
			}
			localSeen[next] = true
			path = append(path, next)
			if found := dfs(next); found != nil {
				return found
			}
			path = path[:len(path)-1]
			delete(localSeen, next)
		}
		return nil
	}
	return dfs(start)
}

// ResolveTargets maps configured target names (node ids or handles) to nodes.
// Empty configuration means all current sink nodes.
func (g *Graph) ResolveTargets() []string {
	names := g.Config.TargetNames()
	var targets []string
	if len(names) == 0 {
		for _, id := range g.IDs {
			if len(g.Edges[id]) == 0 {
				targets = append(targets, id)
			}
		}
		sort.Strings(targets)
		return targets
	}
	seen := map[string]bool{}
	for _, name := range names {
		id, _, ok := g.resolveEndpoint(name)
		if !ok {
			g.errorf("unknown_target", "", "目标输出 %q 既不是节点也不是已声明句柄", name)
			continue
		}
		if !seen[id] {
			seen[id] = true
			targets = append(targets, id)
		}
	}
	sort.Strings(targets)
	return targets
}

// UsedSet returns targets plus every ancestor via reverse edges.
func (g *Graph) UsedSet(targets []string) map[string]bool {
	used := map[string]bool{}
	var walk func(id string)
	walk = func(id string) {
		if used[id] {
			return
		}
		used[id] = true
		for _, p := range sortedKeys(g.Reverse[id]) {
			walk(p)
		}
	}
	for _, t := range targets {
		walk(t)
	}
	return used
}

// Classify computes forks, merges, cycles and unused nodes. It must run
// after ResolveTargets because target errors are appended to diagnostics.
func (g *Graph) Classify(targets []string, order []string, topoErr error) Structure {
	st := Structure{TopologicalOrder: order, Targets: append([]string{}, targets...)}

	if cycleErr, ok := topoErr.(*CycleError); ok {
		st.Cycles = cycleErr.Cycles
	} else if topoErr != nil {
		st.Cycles = g.FindCycles()
	}

	// Forks: group bindings by the exact output socket they consume.
	type fan struct {
		source string
		handle string
		cons   map[string]bool
	}
	fans := map[string]*fan{}
	for _, b := range g.Bindings {
		if !b.Resolved || b.Producer == "" {
			continue
		}
		socket := b.OutputSocket
		key := b.Producer + "\x00" + socket
		f := fans[key]
		if f == nil {
			f = &fan{source: b.Producer, handle: socket, cons: map[string]bool{}}
			fans[key] = f
		}
		f.cons[b.Consumer] = true
	}
	fanKeys := make([]string, 0, len(fans))
	for k := range fans {
		fanKeys = append(fanKeys, k)
	}
	sort.Strings(fanKeys)
	for _, k := range fanKeys {
		f := fans[k]
		if len(f.cons) < 2 {
			continue
		}
		st.Forks = append(st.Forks, Fork{
			Handle: f.handle, Source: f.source,
			Consumers: sortedKeys(f.cons), Legal: true,
		})
	}

	// Merges: distinct predecessor count >= 2.
	for _, id := range g.IDs {
		preds := sortedKeys(g.Reverse[id])
		if len(preds) >= 2 {
			st.Merges = append(st.Merges, Merge{Node: id, Producers: preds, Legal: true})
		}
	}

	used := g.UsedSet(targets)
	for _, id := range g.IDs {
		if used[id] {
			continue
		}
		reason := "不在任何请求目标的反向依赖链上"
		if len(g.Edges[id]) == 0 {
			reason = "末端节点，输出无人消费且未被列为目标"
		} else {
			feedsUsed := false
			for down := range g.Edges[id] {
				if used[down] {
					feedsUsed = true
				}
			}
			if !feedsUsed {
				reason = "只向同样未使用的下游节点供数"
			}
		}
		st.UnusedNodes = append(st.UnusedNodes, UnusedNode{
			ID: id, Type: g.NodeMap[id].Type, Reason: reason,
		})
	}

	referencedHandles := map[string]bool{}
	for _, b := range g.Bindings {
		if b.Handle != "" {
			referencedHandles[b.Handle] = true
		}
	}
	targetHandles := map[string]bool{}
	for _, t := range g.Config.TargetNames() {
		if _, isNode := g.NodeMap[t]; !isNode {
			targetHandles[t] = true
		}
	}
	targetNodes := map[string]bool{}
	for _, t := range targets {
		targetNodes[t] = true
	}
	for handle := range g.Handles {
		decls := g.Handles[handle]
		producerIsTarget := false
		for _, dc := range decls {
			if targetNodes[dc.node.ID] {
				producerIsTarget = true
			}
		}
		if !referencedHandles[handle] && !targetHandles[handle] && !producerIsTarget {
			st.UnusedOutputs = append(st.UnusedOutputs, handle)
		}
	}
	sort.Strings(st.UnusedOutputs)
	return st
}
