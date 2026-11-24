package main

import (
	"encoding/json"
	"strings"
	"testing"
)

func cfgFromJSON(t *testing.T, raw string) *Config {
	t.Helper()
	var cfg Config
	if err := json.Unmarshal([]byte(raw), &cfg); err != nil {
		t.Fatalf("parse config: %v\n%s", err, raw)
	}
	return &cfg
}

const diamondJSON = `{
  "defaultColorSpace": "sRGB",
  "targets": ["out"],
  "nodes": [
    {"id":"src","type":"load","outputs":[{"name":"raw","space":"sRGB"}]},
    {"id":"dec","type":"convert","inputs":["raw"],"outputs":[{"name":"lin","space":"Linear sRGB"}]},
    {"id":"a","type":"gain","inputs":["lin"],"outputs":[{"name":"aL","space":"Linear sRGB"}]},
    {"id":"b","type":"gain","inputs":["lin"],"outputs":[{"name":"bL","space":"Linear sRGB"}]},
    {"id":"join","type":"mix","inputs":["aL","bL"],"outputs":[{"name":"jL","space":"Linear sRGB"}]},
    {"id":"out","type":"encode","inputs":["jL"],"outputs":[{"name":"fin","space":"sRGB"}]}
  ]
}`

func TestTopoSortRespectsEdges(t *testing.T) {
	cfg := cfgFromJSON(t, diamondJSON)
	g := BuildGraph(cfg)
	order, err := g.TopoSort()
	if err != nil {
		t.Fatalf("unexpected cycle: %v", err)
	}
	pos := map[string]int{}
	for i, id := range order {
		pos[id] = i
	}
	for _, edge := range [][2]string{
		{"src", "dec"}, {"dec", "a"}, {"dec", "b"},
		{"a", "join"}, {"b", "join"}, {"join", "out"},
	} {
		if pos[edge[0]] >= pos[edge[1]] {
			t.Fatalf("edge %s->%s violated in order %v", edge[0], edge[1], order)
		}
	}
	if len(order) != 6 {
		t.Fatalf("expected 6 nodes, got %v", order)
	}
}

func TestForkAndMergeAreLegal(t *testing.T) {
	r := BuildReport(cfgFromJSON(t, diamondJSON))
	if !r.Valid {
		t.Fatalf("diamond should be valid: %+v", r.Diagnostics)
	}
	if len(r.Cycles) != 0 {
		t.Fatalf("diamond must have no cycles, got %+v", r.Cycles)
	}
	if len(r.Forks) != 1 {
		t.Fatalf("expected exactly 1 fork, got %+v", r.Forks)
	}
	fork := r.Forks[0]
	if fork.Source != "dec" || len(fork.Consumers) != 2 || !fork.Legal {
		t.Fatalf("bad fork classification: %+v", fork)
	}
	if len(r.Merges) != 1 || r.Merges[0].Node != "join" || !r.Merges[0].Legal {
		t.Fatalf("expected one legal merge at join, got %+v", r.Merges)
	}
	if len(r.UnusedNodes) != 0 {
		t.Fatalf("targeted diamond should have no unused nodes, got %+v", r.UnusedNodes)
	}
}

func TestTrueCycleDetected(t *testing.T) {
	raw := `{
      "targets":["d"],
      "nodes":[
        {"id":"a","type":"gain","inputs":["c"],"outputs":[{"name":"ao"}]},
        {"id":"b","type":"gain","inputs":["ao"],"outputs":[{"name":"bo"}]},
        {"id":"c","type":"gain","inputs":["bo"],"outputs":[{"name":"co"}]},
        {"id":"d","type":"encode","inputs":["co"]}
      ]}`
	r := BuildReport(cfgFromJSON(t, raw))
	if r.Valid {
		t.Fatal("cyclic graph must be invalid")
	}
	if len(r.Cycles) != 1 {
		t.Fatalf("expected 1 cycle, got %+v", r.Cycles)
	}
	cyc := r.Cycles[0]
	joined := strings.Join(cyc.Nodes, ",")
	for _, want := range []string{"a", "b", "c"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("cycle %v missing %s", cyc.Nodes, want)
		}
	}
	if len(cyc.Path) < 4 || cyc.Path[0] != cyc.Path[len(cyc.Path)-1] {
		t.Fatalf("cycle path should close on itself, got %v", cyc.Path)
	}
	_, topoErr := BuildGraph(cfgFromJSON(t, raw)).TopoSort()
	if _, isCycle := topoErr.(*CycleError); !isCycle {
		t.Fatalf("expected *CycleError, got %T", topoErr)
	}
}

func TestSelfLoop(t *testing.T) {
	raw := `{"nodes":[{"id":"loop","type":"gain","inputs":["loop"]}]}`
	r := BuildReport(cfgFromJSON(t, raw))
	if r.Valid || len(r.Cycles) != 1 {
		t.Fatalf("self loop must be one cycle, valid=%v cycles=%+v", r.Valid, r.Cycles)
	}
	if p := r.Cycles[0].Path; len(p) != 2 || p[0] != "loop" || p[1] != "loop" {
		t.Fatalf("bad self-loop path %v", p)
	}
}

func TestDuplicateOutputs(t *testing.T) {
	raw := `{"nodes":[
      {"id":"x","type":"load","outputs":[{"name":"h","space":"sRGB"}]},
      {"id":"y","type":"load","outputs":[{"name":"h","space":"BT.709"}]},
      {"id":"z","type":"encode","inputs":["h"]}]}`
	r := BuildReport(cfgFromJSON(t, raw))
	if r.Valid {
		t.Fatal("duplicate output config must be invalid")
	}
	if len(r.DuplicateOutputs) != 1 || r.DuplicateOutputs[0].Handle != "h" {
		t.Fatalf("missing duplicate report: %+v", r.DuplicateOutputs)
	}
	got := strings.Join(r.DuplicateOutputs[0].DeclaredBy, ",")
	if got != "x,y" {
		t.Fatalf("declaredBy should be [x y], got %v", r.DuplicateOutputs[0].DeclaredBy)
	}
}

func TestUnusedNodesAndDeadBranch(t *testing.T) {
	raw := `{
      "targets":["keep"],
      "nodes":[
        {"id":"src","type":"load","outputs":[{"name":"s"}]},
        {"id":"keep","type":"encode","inputs":["s"]},
        {"id":"dead1","type":"gain","inputs":["s"],"outputs":[{"name":"d1"}]},
        {"id":"dead2","type":"encode","inputs":["d1"]},
        {"id":"orphan","type":"matrix","outputs":[{"name":"o"}]}
      ]}`
	r := BuildReport(cfgFromJSON(t, raw))
	// An inputless non-source node is a warning, not a fatal error.
	unused := map[string]UnusedNode{}
	for _, u := range r.UnusedNodes {
		unused[u.ID] = u
	}
	for _, id := range []string{"dead1", "dead2", "orphan"} {
		if _, ok := unused[id]; !ok {
			t.Fatalf("expected %s unused, got %+v", id, r.UnusedNodes)
		}
	}
	if _, ok := unused["src"]; ok {
		t.Fatal("src is an ancestor of target and must be used")
	}
	if _, ok := unused["keep"]; ok {
		t.Fatal("target keep must be used")
	}
}

func TestMissingReferenceIsError(t *testing.T) {
	raw := `{"targets":["out"],"nodes":[{"id":"out","type":"encode","inputs":["ghost"]}]}`
	r := BuildReport(cfgFromJSON(t, raw))
	if r.Valid {
		t.Fatal("dangling reference must invalidate graph")
	}
	found := false
	for _, d := range r.Diagnostics {
		if d.Code == "unknown_reference" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected unknown_reference diagnostic: %+v", r.Diagnostics)
	}
}

func TestTopoOrderDeterministic(t *testing.T) {
	raw := `{"nodes":[
      {"id":"z","type":"gain","inputs":["root"]},
      {"id":"a","type":"gain","inputs":["root"]},
      {"id":"m","type":"gain","inputs":["root"]},
      {"id":"root","type":"load"}
    ]}`
	var first string
	for i := 0; i < 10; i++ {
		r := BuildReport(cfgFromJSON(t, raw))
		if len(r.TopologicalOrder) != 4 {
			t.Fatalf("bad order %v", r.TopologicalOrder)
		}
		got := strings.Join(r.TopologicalOrder, ",")
		if i == 0 {
			first = got
			if r.TopologicalOrder[0] != "root" {
				t.Fatalf("root must lead, got %v", r.TopologicalOrder)
			}
			if got != "root,a,m,z" {
				t.Fatalf("tie break should be id order, got %v", r.TopologicalOrder)
			}
		} else if got != first {
			t.Fatalf("non-deterministic order: %s vs %s", first, got)
		}
	}
}
