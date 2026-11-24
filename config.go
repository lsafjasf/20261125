package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// LoadConfig reads and parses a JSON graph configuration.
func LoadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("读取图配置失败: %w", err)
	}
	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("解析图配置 JSON 失败 %s: %w", path, err)
	}
	if len(cfg.Nodes) == 0 {
		return nil, fmt.Errorf("图配置 %s 中没有 nodes", path)
	}
	return &cfg, nil
}

// locateRepoFile resolves a repository-relative default such as
// configs/color_graph.json from the working directory or any ancestor, so the
// CLI also works when invoked from a subdirectory.
func locateRepoFile(rel string) string {
	if _, err := os.Stat(rel); err == nil {
		return rel
	}
	dir, err := os.Getwd()
	if err != nil {
		return rel
	}
	for {
		candidate := filepath.Join(dir, rel)
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return rel
		}
		dir = parent
	}
}
