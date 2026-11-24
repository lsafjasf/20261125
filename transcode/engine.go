package transcode

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
)

// Engine executes a deterministic pipeline under a private storage root.
// Separate Engine values may safely point at the same root.
type Engine struct {
	Root     string
	Pipeline []Stage
}

type parameter struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

type stageDescriptor struct {
	Name    string `json:"name"`
	Version string `json:"version"`
	Primary string `json:"primary,omitempty"`
}

type pipelineDescriptor struct {
	Format int               `json:"format"`
	Stages []stageDescriptor `json:"stages"`
}

type jobDescriptor struct {
	Format       int         `json:"format"`
	InputDigest  string      `json:"inputDigest"`
	Parameters   []parameter `json:"parameters"`
	PipelineHash string      `json:"pipelineHash"`
}

type layout struct {
	root string
}

type preparedJob struct {
	layout       layout
	stages       []Stage
	pipelineHash string
	parameters   map[string]string
	inputPath    string
	inputDigest  string
	inputSize    int64
	key          string
}

func (e *Engine) prepare(ctx context.Context, request Request) (*preparedJob, error) {
	if e.Root == "" {
		return nil, fmt.Errorf("%w: storage root is required", ErrInvalidInput)
	}
	if len(e.Pipeline) == 0 {
		return nil, fmt.Errorf("%w: pipeline is empty", ErrInvalidInput)
	}
	if request.InputPath == "" {
		return nil, fmt.Errorf("%w: input path is required", ErrInvalidInput)
	}

	stages, pipelineHash, err := normalizePipeline(e.Pipeline)
	if err != nil {
		return nil, err
	}
	root, err := filepath.Abs(e.Root)
	if err != nil {
		return nil, err
	}
	jobLayout := layout{root: root}
	for _, dir := range []string{
		root,
		filepath.Join(root, "objects", "sha256"),
		filepath.Join(root, "jobs"),
		filepath.Join(root, "work"),
		filepath.Join(root, "locks"),
		filepath.Join(root, "tmp"),
	} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, err
		}
	}

	inputDigest, inputSize, err := hashFile(ctx, request.InputPath)
	if err != nil {
		return nil, err
	}
	parameters := cloneParameters(request.Parameters)
	descriptor := jobDescriptor{
		Format:       formatVersion,
		InputDigest:  inputDigest,
		Parameters:   orderedParameters(parameters),
		PipelineHash: pipelineHash,
	}
	encoded, err := json.Marshal(descriptor)
	if err != nil {
		return nil, err
	}
	return &preparedJob{
		layout:       jobLayout,
		stages:       stages,
		pipelineHash: pipelineHash,
		parameters:   parameters,
		inputPath:    request.InputPath,
		inputDigest:  inputDigest,
		inputSize:    inputSize,
		key:          digestBytes(encoded),
	}, nil
}

func normalizePipeline(pipeline []Stage) ([]Stage, string, error) {
	stages := make([]Stage, len(pipeline))
	descriptors := make([]stageDescriptor, len(pipeline))
	seen := make(map[string]bool, len(pipeline))
	for index, stage := range pipeline {
		if !validStageName(stage.Name) {
			return nil, "", fmt.Errorf("%w: invalid stage name %q", ErrInvalidInput, stage.Name)
		}
		if seen[stage.Name] {
			return nil, "", fmt.Errorf("%w: duplicate stage name %q", ErrInvalidInput, stage.Name)
		}
		if stage.Version == "" {
			return nil, "", fmt.Errorf("%w: stage %q has no version", ErrInvalidInput, stage.Name)
		}
		if stage.Run == nil {
			return nil, "", fmt.Errorf("%w: stage %q has no run function", ErrInvalidInput, stage.Name)
		}
		primary, err := cleanRelative(stage.Primary)
		if err != nil {
			return nil, "", fmt.Errorf("%w: stage %q: %v", ErrInvalidInput, stage.Name, err)
		}
		stage.Primary = primary
		stages[index] = stage
		descriptors[index] = stageDescriptor{
			Name:    stage.Name,
			Version: stage.Version,
			Primary: stage.Primary,
		}
		seen[stage.Name] = true
	}
	encoded, err := json.Marshal(pipelineDescriptor{Format: formatVersion, Stages: descriptors})
	if err != nil {
		return nil, "", err
	}
	return stages, digestBytes(encoded), nil
}

func cloneParameters(parameters map[string]string) map[string]string {
	cloned := make(map[string]string, len(parameters))
	for key, value := range parameters {
		cloned[key] = value
	}
	return cloned
}

func orderedParameters(parameters map[string]string) []parameter {
	keys := make([]string, 0, len(parameters))
	for key := range parameters {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	ordered := make([]parameter, 0, len(keys))
	for _, key := range keys {
		ordered = append(ordered, parameter{Key: key, Value: parameters[key]})
	}
	return ordered
}

func (l layout) shard(kind, digest string) string {
	return filepath.Join(l.root, kind, digest[:2], digest)
}

func (l layout) objectDir(digest string) string {
	return l.shard(filepath.Join("objects", "sha256"), digest)
}

func (l layout) jobWork(key string) string {
	return l.shard("work", key)
}

func (l layout) jobMarker(key string) string {
	return l.shard("jobs", key) + ".json"
}

func (l layout) jobLock(key string) string {
	return filepath.Join(l.root, "locks", "jobs", key+".lock")
}

func (l layout) objectLock(digest string) string {
	return filepath.Join(l.root, "locks", "objects", digest+".lock")
}
