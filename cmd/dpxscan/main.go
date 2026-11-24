// Command dpxscan reconstructs the global frame order of a DPX delivery
// from image headers and manifests, and reports duplicates, gaps,
// timecode regressions, and resolution changes.
package main

import (
	"flag"
	"fmt"
	"os"
	"strings"

	"dpxscan/internal/analyze"
)

type multiFlag []string

func (m *multiFlag) String() string { return strings.Join(*m, ",") }
func (m *multiFlag) Set(v string) error {
	*m = append(*m, v)
	return nil
}

func main() {
	var manifests multiFlag
	var reelOrderCSV string
	var fps int
	var verifyMD5 bool
	flag.Var(&manifests, "manifest", "delivery manifest (md5sum or CSV); repeatable")
	flag.StringVar(&reelOrderCSV, "reel-order", "", "comma-separated explicit reel (directory) order")
	flag.IntVar(&fps, "fps", 0, "frames per second for timecode math (0 = read from headers, default 24)")
	flag.BoolVar(&verifyMD5, "verify-md5", false, "hash each file and compare with the manifest")
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "usage: dpxscan [flags] <delivery-root>\n\n")
		flag.PrintDefaults()
	}
	flag.Parse()
	if flag.NArg() != 1 {
		flag.Usage()
		os.Exit(2)
	}

	opt := analyze.Options{
		FPS:       fps,
		Manifests: manifests,
		VerifyMD5: verifyMD5,
	}
	if reelOrderCSV != "" {
		for _, r := range strings.Split(reelOrderCSV, ",") {
			if r = strings.TrimSpace(r); r != "" {
				opt.ReelOrder = append(opt.ReelOrder, r)
			}
		}
	}

	rep, err := analyze.Scan(flag.Arg(0), opt)
	if err != nil {
		fmt.Fprintln(os.Stderr, "dpxscan:", err)
		os.Exit(1)
	}
	fmt.Print(analyze.FormatText(rep))
}
