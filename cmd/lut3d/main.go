package main

import (
	"flag"
	"fmt"
	"os"

	"lut3d"
)

func main() {
	var (
		red   = flag.Float64("r", 0, "red input value")
		green = flag.Float64("g", 0, "green input value")
		blue  = flag.Float64("b", 0, "blue input value")
	)
	flag.Usage = func() {
		fmt.Fprintf(flag.CommandLine.Output(), "usage: lut3d [-r R -g G -b B] FILE.cube\n")
		flag.PrintDefaults()
	}
	flag.Parse()
	if flag.NArg() != 1 {
		flag.Usage()
		os.Exit(2)
	}

	path := flag.Arg(0)
	lut, err := lut3d.ParseFile(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s: %v\n", path, err)
		os.Exit(1)
	}

	fmt.Printf("valid %q: size=%d entries=%d\n", lut.Title, lut.Size, len(lut.Entries))
	for i, axis := range [3]string{"R", "G", "B"} {
		fmt.Printf("%s domain=[%g,%g] step=%g\n",
			axis, lut.Domain[i].Min, lut.Domain[i].Max, lut.Step[i])
	}
	out := lut.Sample(*red, *green, *blue)
	fmt.Printf("lookup(%g,%g,%g)=(%.12g,%.12g,%.12g)\n",
		*red, *green, *blue, out.R, out.G, out.B)
}
