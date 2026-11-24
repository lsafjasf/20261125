package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/lsafjasf/20261125/transcode"
)

type parameterFlags map[string]string

func (values parameterFlags) String() string {
	return ""
}

func (values parameterFlags) Set(raw string) error {
	parts := strings.SplitN(raw, "=", 2)
	if len(parts) != 2 || parts[0] == "" {
		return fmt.Errorf("parameter must be key=value")
	}
	values[parts[0]] = parts[1]
	return nil
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "transcode:", err)
		os.Exit(1)
	}
}

func run() error {
	if len(os.Args) < 2 || os.Args[1] != "run" {
		return fmt.Errorf("usage: transcode run -root DIR -input FILE [-param k=v]... [-retries N]")
	}
	flags := flag.NewFlagSet("run", flag.ContinueOnError)
	root := flags.String("root", "", "storage root")
	input := flags.String("input", "", "input file")
	retries := flags.Int("retries", 0, "additional attempts after failure")
	parameters := parameterFlags{}
	flags.Var(parameters, "param", "deterministic parameter in key=value form (repeatable)")
	if err := flags.Parse(os.Args[2:]); err != nil {
		return err
	}
	if *root == "" || *input == "" {
		return fmt.Errorf("-root and -input are required")
	}
	if *retries < 0 {
		return fmt.Errorf("-retries cannot be negative")
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	engine := transcode.Engine{
		Root:     *root,
		Pipeline: transcode.PassthroughPipeline(),
	}
	result, err := engine.RunWithRetry(ctx, transcode.Request{
		InputPath:  *input,
		Parameters: parameters,
	}, transcode.RetryPolicy{MaxAttempts: *retries + 1})
	if err != nil {
		return err
	}
	data, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return err
	}
	_, err = fmt.Fprintln(os.Stdout, string(data))
	return err
}
