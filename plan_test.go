package main

import "testing"

func findNodePlan(r *Report, node string) NodePlan {
	for _, np := range r.ColorPlan {
		if np.Node == node {
			return np
		}
	}
	return NodePlan{}
}

func TestEffectiveInputSpaceFromOutputDeclaration(t *testing.T) {
	raw := `{"nodes":[
      {"id":"src","type":"load","outputs":[{"name":"h","space":"BT.709"}]},
      {"id":"n","type":"gain","inputs":[{"from":"h"}]}
    ]}`
	r := BuildReport(cfgFromJSON(t, raw))
	np := findNodePlan(r, "n")
	if len(np.Inputs) != 1 {
		t.Fatalf("expected one input plan: %+v", np)
	}
	ip := np.Inputs[0]
	if ip.AdoptedSpace != "BT.709" || ip.Resolution != "output-declared" {
		t.Fatalf("unexpected input plan: %+v", ip)
	}
}

func TestEffectiveInputSpacePrecedence(t *testing.T) {
	// Undeclared producer output + explicit input assumption.
	raw := `{"defaultColorSpace":"Linear sRGB","nodes":[
      {"id":"src","type":"load","outputs":[{"name":"h"}]},
      {"id":"n","type":"gain","inputs":[{"from":"h","colorSpace":"Display P3"}]}
    ]}`
	r := BuildReport(cfgFromJSON(t, raw))
	ip := findNodePlan(r, "n").Inputs[0]
	if ip.AdoptedSpace != "Display P3" || ip.Resolution != "input-assumption" {
		t.Fatalf("assumption should win over config default: %+v", ip)
	}

	// No assumption -> config default.
	raw2 := `{"defaultColorSpace":"BT.709","nodes":[
      {"id":"src","type":"load","outputs":[{"name":"h"}]},
      {"id":"n","type":"gain","inputs":["h"]}
    ]}`
	r2 := BuildReport(cfgFromJSON(t, raw2))
	ip2 := findNodePlan(r2, "n").Inputs[0]
	if ip2.AdoptedSpace != "BT.709" || ip2.Resolution != "config-default" {
		t.Fatalf("config default should apply: %+v", ip2)
	}

	// Helper-level built-in fallback.
	b := &Binding{}
	space, resolution := b.incomingSpace("")
	if space != "sRGB" || resolution != "built-in-default" {
		t.Fatalf("built-in fallback wrong: %s %s", space, resolution)
	}
}

func TestExpectTriggersImplicitConversion(t *testing.T) {
	raw := `{"nodes":[
      {"id":"src","type":"load","outputs":[{"name":"h","space":"sRGB"}]},
      {"id":"dec","type":"convert","inputs":[{"from":"h","expect":"Linear sRGB"}],
       "outputs":[{"name":"l","space":"Linear sRGB"}]}
    ]}`
	r := BuildReport(cfgFromJSON(t, raw))
	if !r.Valid {
		t.Fatalf("autoConvert should keep graph valid: %+v", r.Diagnostics)
	}
	ip := findNodePlan(r, "dec").Inputs[0]
	if !ip.AutoConverted || ip.AdoptedSpace != "Linear sRGB" || ip.Conversion == "" {
		t.Fatalf("expected planned sRGB->Linear conversion: %+v", ip)
	}
}

func TestMismatchRejectedWhenAutoConvertOff(t *testing.T) {
	off := false
	cfg := cfgFromJSON(t, `{"nodes":[
      {"id":"src","type":"load","outputs":[{"name":"h","space":"sRGB"}]},
      {"id":"grade","type":"gain","inputs":[{"from":"h","expect":"Linear sRGB"}]}
    ]}`)
	cfg.AutoConvert = &off
	r := BuildReport(cfg)
	if r.Valid {
		t.Fatal("declared mismatch with autoConvert=false must invalidate")
	}
	foundMismatch := false
	for _, d := range r.Diagnostics {
		if d.Code == "color_space_mismatch" {
			foundMismatch = true
		}
	}
	if !foundMismatch {
		t.Fatalf("expected color_space_mismatch: %+v", r.Diagnostics)
	}
}

func TestInvalidColorSpaceReported(t *testing.T) {
	raw := `{"nodes":[
      {"id":"src","type":"load","outputs":[{"name":"h","space":"ACEScct"}]},
      {"id":"n","type":"gain","inputs":["h"]}
    ]}`
	r := BuildReport(cfgFromJSON(t, raw))
	if r.Valid {
		t.Fatal("unknown color space must invalidate")
	}
	found := false
	for _, d := range r.Diagnostics {
		if d.Code == "invalid_color_space" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected invalid_color_space diag: %+v", r.Diagnostics)
	}
}

func TestMergeNodeGetsCommonWorkingSpace(t *testing.T) {
	raw := `{"nodes":[
      {"id":"s1","type":"load","outputs":[{"name":"h1","space":"sRGB"}]},
      {"id":"s2","type":"load","outputs":[{"name":"h2","space":"Linear sRGB"}]},
      {"id":"m","type":"mix","inputs":[{"from":"h1"},{"from":"h2"}]}
    ]}`
	r := BuildReport(cfgFromJSON(t, raw))
	np := findNodePlan(r, "m")
	if np.WorkingSpace != "sRGB" {
		t.Fatalf("merge should adopt first input space as common, got %q", np.WorkingSpace)
	}
	if len(np.Inputs) != 2 || !np.Inputs[1].AutoConverted {
		t.Fatalf("second input should be planned for conversion: %+v", np.Inputs)
	}
}
