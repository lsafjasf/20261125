// cutcheck proves that two export files in different formats originate
// from the same canonical edit list, localizes timing discrepancies to a
// format+layer, and re-tests after export fixes.
package main

import (
	"cutcheck/internal/cmx"
	"cutcheck/internal/compare"
	"cutcheck/internal/fcpxml"
	"cutcheck/internal/pipeline"
	"cutcheck/internal/timeline"
	"fmt"
	"os"
)

func main() {
	if len(os.Args) >= 2 && os.Args[1] == "verify" {
		os.Exit(cmdVerify(os.Args[2:]))
	}
	os.Exit(cmdRun())
}

// cmdRun executes the full release-gate pipeline:
// export with current (broken) parameters -> reopen -> diagnose ->
// fix export parameters -> re-export -> reopen -> re-test.
func cmdRun() int {
	el, err := timeline.Load("editlist.json")
	if err != nil {
		fmt.Fprintln(os.Stderr, "load edit list:", err)
		return 2
	}
	declared := pipeline.DeclaredNonEquivalents(el)
	fmt.Printf("canonical edit list: %s (%d video, %d audio events, %d/%d fps, %d Hz)\n",
		el.Title, len(el.Video), len(el.Audio), el.FPSNum, el.FPSDen, el.SampleRate)

	// Phase 1: reproduce the current exports with their misconfigured
	// parameters: CMX written at 30/1 instead of 30000/1001, FCPXML audio
	// carrying 1024 samples of uncompensated codec priming.
	fmt.Println("\n=== PHASE 1: export with current parameters ===")
	buggyCMX := cmx.CorrectConfig(el)
	buggyCMX.FPSNum, buggyCMX.FPSDen = 30, 1 // misconfigured export rate
	buggyFCP := fcpxml.ExportConfig{AudioPrimingSamples: 1024}
	cmxPath, fcpPath, warnings, err := pipeline.ExportPair(el, "out/current", buggyCMX, buggyFCP)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 2
	}
	fmt.Printf("wrote %s and %s\n", cmxPath, fcpPath)
	for _, w := range warnings {
		fmt.Println("  export warning:", w)
	}

	// Phase 2: reopen and diagnose.
	fmt.Println("\n=== PHASE 2: reopen both exports, build cross-format map, diagnose ===")
	before, err := pipeline.ReopenAndCheck(el, cmxPath, fcpPath, declared)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 2
	}
	printReport(el, before)

	// Phase 3: fix the export parameters and re-export.
	fmt.Println("\n=== PHASE 3: fix export parameters, re-export ===")
	fmt.Println("  fix 1: CMX export rate 30/1 -> 30000/1001 (drop-frame)")
	fmt.Println("  fix 2: FCPXML audio priming 1024 -> 0 samples (compensated)")
	cmxPath2, fcpPath2, _, err := pipeline.ExportPair(el, "out/fixed",
		cmx.CorrectConfig(el), fcpxml.CorrectConfig(el))
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 2
	}
	fmt.Printf("wrote %s and %s\n", cmxPath2, fcpPath2)

	// Phase 4: reopen the fixed exports and re-test.
	fmt.Println("\n=== PHASE 4: reopen fixed exports, re-test equivalence ===")
	after, err := pipeline.ReopenAndCheck(el, cmxPath2, fcpPath2, declared)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 2
	}
	printReport(el, after)
	printDeclared(declared)

	if len(after.Failures) > 0 {
		fmt.Println("\nRESULT: FAIL - exports are not equivalent to the edit list")
		return 1
	}
	fmt.Println("\nRESULT: PASS - both exports derive from the same edit list (declared exceptions listed)")
	return 0
}

// cmdVerify re-opens two existing exports and re-tests equivalence:
// cutcheck verify <editlist.json> <file.cmx.edl> <file.fcpxml>
func cmdVerify(args []string) int {
	if len(args) != 3 {
		fmt.Fprintln(os.Stderr, "usage: cutcheck verify <editlist.json> <cmx.edl> <fcpxml>")
		return 2
	}
	el, err := timeline.Load(args[0])
	if err != nil {
		fmt.Fprintln(os.Stderr, "load edit list:", err)
		return 2
	}
	declared := pipeline.DeclaredNonEquivalents(el)
	res, err := pipeline.ReopenAndCheck(el, args[1], args[2], declared)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 2
	}
	printReport(el, res)
	printDeclared(declared)
	if len(res.Failures) > 0 {
		fmt.Println("\nRESULT: FAIL")
		return 1
	}
	fmt.Println("\nRESULT: PASS")
	return 0
}

func printReport(el *timeline.EditList, res *pipeline.PhaseResult) {
	fmt.Println("  cross-format event map (event -> found in cmx/fcpxml):")
	for _, row := range res.Mapping {
		fmt.Printf("    %-4s %-6s cmx=%-5v fcpxml=%-5v\n",
			row.EventID, row.Layer, row.Found["cmx"], row.Found["fcpxml"])
	}
	fmt.Println("  layer diagnosis:")
	for _, d := range res.Diagnoses {
		if d.Pattern == compare.PatternNone {
			fmt.Printf("    %-6s/%-5s OK (max |delta| %.2f %s)\n", d.Format, d.Layer, d.MaxAbs, d.Unit)
			continue
		}
		fmt.Printf("    %-6s/%-5s %s: max |delta| %.2f %s\n", d.Format, d.Layer, d.Pattern, d.MaxAbs, d.Unit)
		fmt.Println("      ->", d.Detail)
	}
	if len(res.Failures) > 0 {
		fmt.Println("  equivalence failures:")
		for _, f := range res.Failures {
			fmt.Println("    FAIL:", f)
		}
	} else {
		fmt.Println("  equivalence: all mapped events within tolerance")
	}
}

func printDeclared(declared []compare.NonEquivalent) {
	fmt.Println("\n  declared non-equivalent items (excluded from proof):")
	for _, d := range declared {
		if d.EventID == "" {
			fmt.Printf("    [%s] (format-wide) %s\n", d.Format, d.Reason)
		} else {
			fmt.Printf("    [%s] event %s: %s\n", d.Format, d.EventID, d.Reason)
		}
	}
}
