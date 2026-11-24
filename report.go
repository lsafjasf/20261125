package main

import (
	"fmt"
	"io"
	"strings"
)

func writeReport(writer io.Writer, report ConversionReport) error {
	var builder strings.Builder
	fmt.Fprintf(&builder, "Source fps: %s\n", report.SourceRate.RatString())

	for _, step := range report.Steps {
		fmt.Fprintf(&builder, "\nStep %d: %s fps -> %s fps (factor %s, rounding %s)\n",
			step.Number, step.SourceRate.RatString(), step.TargetRate.RatString(),
			step.Factor.RatString(), step.Rounding)
		builder.WriteString("cue\tbound\texact_ms\tquantized_ms\tquantization_error_ms\n")
		for _, quant := range step.Quantization {
			fmt.Fprintf(&builder, "%d\t%s\t%s\t%s\t%s\n",
				quant.CueIndex,
				quant.Bound,
				quant.Exact.RatString(),
				quant.Quantized.RatString(),
				quant.MillisecondErr.RatString(),
			)
		}
	}

	builder.WriteString("\nValidation\n")
	if len(report.Issues) == 0 {
		builder.WriteString("no issues\n")
	} else {
		for _, issue := range report.Issues {
			fmt.Fprintf(&builder, "stage=%d\tcue=%d\t%s\t%s\t%s\n",
				issue.Stage, issue.CueIndex, issue.Severity, issue.Kind, issue.Message)
		}
	}

	if report.RoundTrip.Available {
		builder.WriteString("\nRound trip\n")
		fmt.Fprintf(&builder, "min_delta_ms: %s\n", report.RoundTrip.MinDelta.RatString())
		fmt.Fprintf(&builder, "max_delta_ms: %s\n", report.RoundTrip.MaxDelta.RatString())
		if report.RoundTrip.ConstantOffset {
			fmt.Fprintf(&builder, "global_offset_correction: possible with %s ms\n",
				report.RoundTrip.SharedOffset.RatString())
		} else {
			builder.WriteString("global_offset_correction: impossible; deltas are not constant\n")
		}
	}

	_, err := io.WriteString(writer, builder.String())
	return err
}
