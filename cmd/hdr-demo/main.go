package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"capmatrix/display"
)

type swatchResult struct {
	Swatch      string     `json:"swatch"`
	InputLinear [3]float64 `json:"inputLinearSRGBRelative"`
	Signal      [3]float64 `json:"normalizedSignal"`
	CodeValues  [3]int     `json:"codeValues"`
	CodeMax     int        `json:"codeMax"`
}

type deviceReport struct {
	DisplayID       string                   `json:"displayId"`
	ConfigValid     bool                     `json:"configValid"`
	SelectedTier    display.TierID           `json:"selectedTier"`
	Transfer        display.TransferFunction `json:"transferFunction"`
	PeakNits        float64                  `json:"peakNits"`
	BlackNits       float64                  `json:"blackNits"`
	Bits            int                      `json:"bits"`
	Fallback        bool                     `json:"fallback"`
	FallbackReasons []string                 `json:"fallbackReasons,omitempty"`
	Attempts        []display.Attempt        `json:"matrixAttempts"`
	Outputs         []swatchResult           `json:"outputs"`
}

type report struct {
	SourceGamut string         `json:"sourceGamut"`
	Devices     []deviceReport `json:"devices"`
}

func main() {
	src, ok := display.StandardGamut("sRGB")
	if !ok {
		panic("sRGB source gamut unavailable")
	}
	material := sourceMaterial()
	rep := report{SourceGamut: "sRGB (D65), linear, 100-nit reference white"}

	for _, d := range simulatedDevices() {
		dec := display.Resolve(d, src)
		dr := deviceReport{
			DisplayID:       d.ID,
			ConfigValid:     dec.ConfigValid,
			SelectedTier:    dec.Selected,
			Transfer:        dec.Profile.Transfer,
			PeakNits:        dec.Profile.PeakNits,
			BlackNits:       dec.Profile.BlackNits,
			Bits:            dec.Profile.Bits,
			Fallback:        dec.Fallback,
			FallbackReasons: dec.FallbackReasons,
			Attempts:        dec.Attempts,
		}
		for _, px := range material {
			signal, codes := display.ConvertPixel(px.RGB, dec.Profile)
			dr.Outputs = append(dr.Outputs, swatchResult{
				Swatch:      px.Name,
				InputLinear: px.RGB,
				Signal:      roundTriple(signal),
				CodeValues:  codes,
				CodeMax:     dec.Profile.CodeMax,
			})
		}
		rep.Devices = append(rep.Devices, dr)
	}

	printHuman(rep)
	if err := writeJSON(rep); err != nil {
		fmt.Fprintf(os.Stderr, "failed to write JSON report: %v\n", err)
		os.Exit(1)
	}
}

func printHuman(rep report) {
	fmt.Println("== Capability-matrix conversion decisions ==")
	for _, d := range rep.Devices {
		fmt.Printf("\n[%s] valid=%v tier=%s transfer=%s peak=%.2f nits black=%.3f nits bits=%d\n",
			d.DisplayID, d.ConfigValid, d.SelectedTier, d.Transfer, d.PeakNits, d.BlackNits, d.Bits)
		for _, a := range d.Attempts {
			tag := "eligible"
			if !a.Eligible {
				tag = "skipped"
			}
			fmt.Printf("    - %-15s %-8s %s", a.Tier, tag, a.Reason)
			if a.VerifyError != "" {
				fmt.Printf(" | verify error: %s", a.VerifyError)
			}
			fmt.Println()
		}
		if d.Fallback {
			fmt.Println("    FALLBACK to verifiable safe tier because:")
			for _, r := range d.FallbackReasons {
				fmt.Printf("      * %s\n", r)
			}
		}
	}

	fmt.Println("\n== Same material, device-specific outputs (R G B code values) ==")
	header := "swatch"
	for _, d := range rep.Devices {
		header += fmt.Sprintf(" | %-18s", d.DisplayID+":"+string(d.SelectedTier))
	}
	fmt.Println(header)
	for i := range rep.Devices[0].Outputs {
		line := fmt.Sprintf("%-12s", rep.Devices[0].Outputs[i].Swatch)
		for _, d := range rep.Devices {
			c := d.Outputs[i].CodeValues
			line += fmt.Sprintf(" | %3d,%3d,%3d/%-5d", c[0], c[1], c[2], d.Outputs[i].CodeMax)
		}
		fmt.Println(line)
	}
}

func writeJSON(rep report) error {
	out, err := json.MarshalIndent(rep, "", "  ")
	if err != nil {
		return err
	}
	path := filepath.Join("artifacts", "conversion-report.json")
	return os.WriteFile(path, append(out, '\n'), 0o644)
}

func roundTriple(v [3]float64) [3]float64 {
	for i := range v {
		v[i] = float64(int(v[i]*1e6+0.5)) / 1e6
	}
	return v
}
