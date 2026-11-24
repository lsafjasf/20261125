package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"

	"otioflat"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "otioflat:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) < 2 {
		return errors.New("usage: otioflat <flatten|unflatten|export-otio|import-otio> <input.json> [output.json]")
	}
	command, inputPath := args[0], args[1]
	input, err := os.ReadFile(inputPath)
	if err != nil {
		return err
	}

	var output []byte
	switch command {
	case "flatten":
		var timeline otioflat.Timeline
		if err := json.Unmarshal(input, &timeline); err != nil {
			return err
		}
		output, err = otioflat.FlattenJSON(&timeline)
	case "unflatten":
		timeline, decodeErr := otioflat.UnflattenJSON(input)
		if decodeErr != nil {
			return decodeErr
		}
		output, err = json.MarshalIndent(timeline, "", "  ")
	case "export-otio":
		var timeline otioflat.Timeline
		if err := json.Unmarshal(input, &timeline); err != nil {
			return err
		}
		output, err = otioflat.FlattenOTIOJSON(&timeline)
	case "import-otio":
		timeline, decodeErr := otioflat.UnflattenOTIOJSON(input)
		if decodeErr != nil {
			return decodeErr
		}
		output, err = json.MarshalIndent(timeline, "", "  ")
	default:
		return fmt.Errorf("unknown command %q", command)
	}
	if err != nil {
		return err
	}
	output = append(output, '\n')

	if len(args) >= 3 {
		return os.WriteFile(args[2], output, 0o644)
	}
	_, err = os.Stdout.Write(output)
	return err
}
