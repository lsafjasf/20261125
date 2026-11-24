package main

import "strings"

// normalizeOp folds aliases and separators so "ChannelMix", "channel_mix"
// and "channelmix" land on the same operator name.
func normalizeOp(t string) string {
	s := strings.ToLower(strings.TrimSpace(t))
	var b strings.Builder
	for _, r := range s {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	s = b.String()
	switch s {
	case "colourspace", "color", "cs", "spacetransform":
		return "convert"
	case "colourconvert", "colorconvert", "conversion":
		return "convert"
	case "exposure", "multiply", "scale":
		return "gain"
	case "channelmixer", "colormatrix", "matrixmix":
		return "matrix"
	case "blend", "composite", "mergenodes":
		return "mix"
	case "writer", "output", "save", "png", "render":
		return "encode"
	case "source", "reader", "input", "file":
		return "load"
	case "null", "noop", "terminal", "sinknode":
		return "sink"
	}
	return s
}

func paramString(params map[string]any, keys ...string) string {
	for _, k := range keys {
		if v, ok := params[k]; ok {
			if s, ok := v.(string); ok && s != "" {
				return s
			}
		}
	}
	return ""
}

func paramFloat(params map[string]any, def float64, keys ...string) float64 {
	for _, k := range keys {
		if v, ok := params[k]; ok {
			if f, ok := toFloat(v); ok {
				return f
			}
		}
	}
	return def
}

func paramBool(params map[string]any, def bool, keys ...string) bool {
	for _, k := range keys {
		if v, ok := params[k]; ok {
			if b, ok := v.(bool); ok {
				return b
			}
		}
	}
	return def
}

func toFloat(v any) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case float32:
		return float64(n), true
	case int:
		return float64(n), true
	case int64:
		return float64(n), true
	}
	return 0, false
}

func paramVec3(params map[string]any, def RGB, keys ...string) RGB {
	for _, k := range keys {
		v, ok := params[k]
		if !ok {
			continue
		}
		if arr, ok := v.([]any); ok && len(arr) >= 3 {
			r, _ := toFloat(arr[0])
			g, _ := toFloat(arr[1])
			b, _ := toFloat(arr[2])
			return RGB{r, g, b}
		}
	}
	return def
}

func paramWeights(params map[string]any, n int) []float64 {
	w := make([]float64, n)
	for i := range w {
		w[i] = 1
	}
	if arr, ok := params["weights"].([]any); ok {
		for i := 0; i < n && i < len(arr); i++ {
			if f, ok := toFloat(arr[i]); ok {
				w[i] = f
			}
		}
	}
	return w
}

func paramMat3(params map[string]any, key string) (mat3, bool) {
	v, ok := params[key]
	if !ok {
		return mat3{}, false
	}
	rows, ok := v.([]any)
	if !ok {
		return mat3{}, false
	}
	if len(rows) == 9 { // flat 9-element array
		var m mat3
		for i := range m {
			f, _ := toFloat(rows[i])
			m[i] = f
		}
		return m, true
	}
	if len(rows) == 3 {
		var m mat3
		idx := 0
		for _, row := range rows {
			cols, ok := row.([]any)
			if !ok || len(cols) != 3 {
				return mat3{}, false
			}
			for _, c := range cols {
				f, _ := toFloat(c)
				m[idx] = f
				idx++
			}
		}
		return m, true
	}
	return mat3{}, false
}
