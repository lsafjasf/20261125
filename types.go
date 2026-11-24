package main

import (
	"encoding/json"
	"sort"
)

// stringList unmarshals from either a single string or an array of strings.
type stringList []string

func (s *stringList) UnmarshalJSON(data []byte) error {
	var single string
	if err := json.Unmarshal(data, &single); err == nil {
		*s = stringList{single}
		return nil
	}
	var many []string
	if err := json.Unmarshal(data, &many); err != nil {
		return err
	}
	*s = many
	return nil
}

// Config is the color conversion graph configuration (the "图配置").
type Config struct {
	Version          int        `json:"version"`
	Name             string     `json:"name"`
	Description      string     `json:"description"`
	DefaultSpace     string     `json:"defaultColorSpace"`
	AutoConvert      *bool      `json:"autoConvert"`
	Frames           string     `json:"frames"`
	FrameGlob        string     `json:"frameGlob"`
	Targets          stringList `json:"targets"`
	Outputs          stringList `json:"outputs"`
	RequestedOutputs stringList `json:"requestedOutputs"`
	Nodes            []*Node    `json:"nodes"`
	Edges            []EdgeSpec `json:"edges"`
}

// UnmarshalJSON accepts common snake_case aliases for graph-level settings.
func (c *Config) UnmarshalJSON(data []byte) error {
	type configCodec Config
	var p struct {
		configCodec
		Nodes rawNodes `json:"nodes"`
	}
	if err := json.Unmarshal(data, &p); err != nil {
		return err
	}
	*c = Config(p.configCodec)
	c.Nodes = []*Node(p.Nodes)
	var aliases struct {
		DefaultSpace     string `json:"default_space"`
		DefaultInputCS   string `json:"defaultInputColorSpace"`
		DefaultCS        string `json:"defaultCS"`
		FrameDir         string `json:"frameDir"`
		FramesDir        string `json:"framesDir"`
		FramePattern     string `json:"framePattern"`
		AutoConvertSnake *bool  `json:"auto_convert"`
	}
	if err := json.Unmarshal(data, &aliases); err != nil {
		return err
	}
	c.DefaultSpace = firstNonEmpty(c.DefaultSpace, aliases.DefaultSpace, aliases.DefaultInputCS, aliases.DefaultCS)
	c.Frames = firstNonEmpty(c.Frames, aliases.FrameDir, aliases.FramesDir)
	c.FrameGlob = firstNonEmpty(c.FrameGlob, aliases.FramePattern)
	if c.AutoConvert == nil && aliases.AutoConvertSnake != nil {
		c.AutoConvert = aliases.AutoConvertSnake
	}
	return nil
}

// rawNodes accepts either an array of nodes or an id->node mapping.
type rawNodes []*Node

func (n *rawNodes) UnmarshalJSON(data []byte) error {
	var arr []*Node
	if err := json.Unmarshal(data, &arr); err == nil {
		*n = arr
		return nil
	}
	var byID map[string]*Node
	if err := json.Unmarshal(data, &byID); err != nil {
		return err
	}
	ids := make([]string, 0, len(byID))
	for id := range byID {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		node := byID[id]
		if node.ID == "" {
			node.ID = id
		}
		*n = append(*n, node)
	}
	return nil
}

// TargetNames merges the several accepted spellings of requested outputs.
func (c *Config) TargetNames() []string {
	var out []string
	out = append(out, c.Targets...)
	out = append(out, c.RequestedOutputs...)
	out = append(out, c.Outputs...)
	return out
}

// FramesTarget returns the configured frame directory/glob, if any.
func (c *Config) FramesTarget() string {
	if c.Frames != "" {
		return c.Frames
	}
	return c.FrameGlob
}

func (c *Config) AutoConvertEnabled() bool {
	if c.AutoConvert == nil {
		return true
	}
	return *c.AutoConvert
}

// Node is one color operation in the graph.
type Node struct {
	ID      string         `json:"id"`
	Type    string         `json:"type"`
	Inputs  []InputSpec    `json:"inputs"`
	Outputs []OutputSpec   `json:"outputs"`
	Params  map[string]any `json:"params"`
	Index   int            `json:"-"`
}

// OutputSpec is a declared output handle plus the space it emits.
type OutputSpec struct {
	Name  string `json:"name"`
	Space string `json:"space"`
}

// InputSpec describes one incoming wire. Ref may be an output handle or a
// producer node id.
type InputSpec struct {
	Name   string `json:"name"`
	Ref    string `json:"ref"`
	Socket string `json:"socket"`
	Space  string `json:"space"`
	Expect string `json:"expect"`
}

// EdgeSpec is an explicitly declared edge; endpoints accept node ids or handles.
type EdgeSpec struct {
	From string `json:"from"`
	To   string `json:"to"`
}

var knownNodeKeys = map[string]bool{
	"id": true, "name": true, "nodeid": true,
	"type": true, "op": true, "kind": true, "operator": true,
	"inputs": true, "input": true, "in": true,
	"deps": true, "dependencies": true, "requires": true,
	"outputs": true, "output": true, "out": true,
	"provides": true, "produces": true,
	"params": true, "parameters": true, "args": true,
}

// UnmarshalJSON accepts both {"id":"a"} and {"name":"a"} style nodes and
// folds any unknown keys into Params so flattened operation settings work.
func (n *Node) UnmarshalJSON(data []byte) error {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return err
	}
	n.Params = map[string]any{}
	get := func(keys ...string) json.RawMessage {
		for _, k := range keys {
			if raw, ok := fields[k]; ok {
				return raw
			}
		}
		return nil
	}
	if raw := get("id", "nodeId", "nodeID"); raw != nil {
		_ = json.Unmarshal(raw, &n.ID)
	}
	if n.ID == "" {
		_ = json.Unmarshal(get("name"), &n.ID)
	}
	if raw := get("type", "op", "kind", "operator"); raw != nil {
		_ = json.Unmarshal(raw, &n.Type)
	}
	if raw := get("inputs", "input", "in", "deps", "dependencies", "requires"); raw != nil {
		n.Inputs = decodeInputList(raw)
	}
	if raw := get("outputs", "output", "out", "provides", "produces"); raw != nil {
		n.Outputs = decodeOutputList(raw)
	}
	if raw := get("params", "parameters", "args"); raw != nil {
		_ = json.Unmarshal(raw, &n.Params)
	}
	for key, raw := range fields {
		if knownNodeKeys[key] {
			continue
		}
		var value any
		if err := json.Unmarshal(raw, &value); err == nil {
			n.Params[key] = value
		}
	}
	return nil
}

func decodeInputList(raw json.RawMessage) []InputSpec {
	var many []InputSpec
	if err := json.Unmarshal(raw, &many); err == nil {
		return many
	}
	var one InputSpec
	if err := json.Unmarshal(raw, &one); err == nil {
		return []InputSpec{one}
	}
	return nil
}

func decodeOutputList(raw json.RawMessage) []OutputSpec {
	var many []OutputSpec
	if err := json.Unmarshal(raw, &many); err == nil {
		return many
	}
	var one OutputSpec
	if err := json.Unmarshal(raw, &one); err == nil {
		return []OutputSpec{one}
	}
	return nil
}

func (o *OutputSpec) UnmarshalJSON(data []byte) error {
	var text string
	if err := json.Unmarshal(data, &text); err == nil {
		o.Name = text
		return nil
	}
	var obj struct {
		Name       string `json:"name"`
		ID         string `json:"id"`
		Handle     string `json:"handle"`
		Key        string `json:"key"`
		Space      string `json:"space"`
		CS         string `json:"colorSpace"`
		ColorSpace string `json:"color_space"`
	}
	if err := json.Unmarshal(data, &obj); err != nil {
		return err
	}
	o.Name = firstNonEmpty(obj.Name, obj.Handle, obj.ID, obj.Key)
	o.Space = firstNonEmpty(obj.Space, obj.CS, obj.ColorSpace)
	return nil
}

func (in *InputSpec) UnmarshalJSON(data []byte) error {
	var text string
	if err := json.Unmarshal(data, &text); err == nil {
		in.Ref = text
		return nil
	}
	var obj struct {
		Ref       string `json:"ref"`
		From      string `json:"from"`
		Source    string `json:"source"`
		Output    string `json:"output"`
		Handle    string `json:"handle"`
		Producer  string `json:"producer"`
		Node      string `json:"node"`
		Name      string `json:"name"`
		InputName string `json:"inputName"`
		Label     string `json:"label"`
		Socket    string `json:"socket"`
		OutSocket string `json:"outputName"`
		Space     string `json:"space"`
		CS        string `json:"colorSpace"`
		Assume    string `json:"assumeSpace"`
		AssumeCS  string `json:"assume"`
		Expect    string `json:"expect"`
		Target    string `json:"targetSpace"`
		Working   string `json:"workingSpace"`
		Required  string `json:"requiredSpace"`
	}
	if err := json.Unmarshal(data, &obj); err != nil {
		return err
	}
	in.Ref = firstNonEmpty(obj.Ref, obj.From, obj.Source, obj.Output, obj.Handle, obj.Producer, obj.Node)
	in.Socket = firstNonEmpty(obj.Socket, obj.OutSocket)
	in.Name = firstNonEmpty(obj.Name, obj.InputName, obj.Label)
	in.Space = firstNonEmpty(obj.Space, obj.CS, obj.Assume, obj.AssumeCS)
	in.Expect = firstNonEmpty(obj.Expect, obj.Target, obj.Working, obj.Required)
	return nil
}

func (e *EdgeSpec) UnmarshalJSON(data []byte) error {
	var pair [2]string
	if err := json.Unmarshal(data, &pair); err == nil {
		e.From, e.To = pair[0], pair[1]
		return nil
	}
	var obj struct {
		From   string `json:"from"`
		Source string `json:"source"`
		Tail   string `json:"tail"`
		To     string `json:"to"`
		Target string `json:"target"`
		Head   string `json:"head"`
	}
	if err := json.Unmarshal(data, &obj); err != nil {
		return err
	}
	e.From = firstNonEmpty(obj.From, obj.Source, obj.Tail)
	e.To = firstNonEmpty(obj.To, obj.Target, obj.Head)
	return nil
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}
