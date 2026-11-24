package main

import (
	"flag"
	"fmt"
	"io"
)

type options struct {
	Out          string
	Input        string
	Width        int
	Height       int
	Frames       int
	FixedKnee    float64
	Window       int
	MaxSlew      float64
	Bit16        bool
	ContactFrame int
	Transfer     string
	SignalRange  string
	Exposure     float64
	PreserveHue  bool
	SaveSource   bool
}

func printUsage(w io.Writer) {
	fmt.Fprint(w, `rec709shoulder — Rec.2020 vertical master to Rec.709 highlight comparisons

Usage:
  rec709shoulder all [flags]       Generate diagnostic source, three versions, metrics and sheets
  rec709shoulder demo [flags]      Generate Rec.2020 PFM diagnostic frames only
  rec709shoulder process [flags]   Process PNG/JPEG/PFM frame sequence
  rec709shoulder help

Core flags:
  -input DIR          Input frame directory for process
  -out DIR            Output root (default outputs)
  -frames N           Demo frame count (default 12)
  -width N -height N  Demo dimensions (default 540x960)
  -fixed-knee F       Fixed shoulder knee (default 0.75)
  -stable-window N    Temporal median odd window >=3 (default 5)
  -max-knee-slew F    Knee rate limit per frame (default 0.035)
  -transfer NAME      rec709|srgb|linear|pq for PNG/JPEG input (default rec709)
  -range full|limited Input signal range (default full)
  -exposure F         Linear gain after decode; 0 = auto (100 for PQ, 1 otherwise)
  -hue-preserve       Preserve RGB ratios in soft shoulder (default true; use -hue-preserve=false for per-channel)
  -8bit               Write 8-bit PNG instead of 16-bit
`)
}

func bindFlags(name string, args []string) (*options, *flag.FlagSet, error) {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	o := &options{}
	fs.StringVar(&o.Out, "out", "outputs", "output root")
	fs.StringVar(&o.Input, "input", "", "input frame directory")
	fs.IntVar(&o.Width, "width", 540, "demo width")
	fs.IntVar(&o.Height, "height", 960, "demo height")
	fs.IntVar(&o.Frames, "frames", 12, "demo frames")
	fs.Float64Var(&o.FixedKnee, "fixed-knee", .75, "fixed shoulder knee")
	fs.IntVar(&o.Window, "stable-window", 5, "temporal median window")
	fs.Float64Var(&o.MaxSlew, "max-knee-slew", .035, "maximum knee slew per frame")
	fs.BoolVar(&o.Bit16, "16bit", true, "write 16-bit PNG")
	fs.IntVar(&o.ContactFrame, "contact-frame", -1, "contact sheet frame; -1 uses final frame")
	fs.StringVar(&o.Transfer, "transfer", "rec709", "input transfer curve")
	fs.StringVar(&o.SignalRange, "range", "full", "input signal range")
	fs.Float64Var(&o.Exposure, "exposure", 0, "linear exposure gain; zero selects auto")
	fs.BoolVar(&o.PreserveHue, "hue-preserve", true, "preserve RGB ratios in shoulder")
	fs.BoolVar(&o.SaveSource, "save-source", true, "write generated PFM source frames")
	eightBit := false
	fs.BoolVar(&eightBit, "8bit", false, "write 8-bit PNG instead of 16-bit")
	if err := fs.Parse(args); err != nil {
		return o, fs, err
	}
	if eightBit {
		o.Bit16 = false
	}
	return o, fs, nil
}
