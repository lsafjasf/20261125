package transcode

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

type probeMetadata struct {
	Format      int         `json:"format"`
	InputDigest string      `json:"inputDigest"`
	InputSize   int64       `json:"inputSize"`
	Parameters  []parameter `json:"parameters"`
}

// PassthroughPipeline is a deterministic, dependency-free example pipeline.
// Replace the transcode stage's Run function with a real codec while keeping
// its Name/Version stable only for byte-identical behavior.
func PassthroughPipeline() []Stage {
	return []Stage{
		{
			Name:    "probe",
			Version: "1",
			Primary: "metadata.json",
			Run: func(ctx context.Context, input StageInput) error {
				metadata := probeMetadata{
					Format:      formatVersion,
					InputDigest: input.InputDigest,
					InputSize:   input.InputSize,
					Parameters:  orderedParameters(input.Parameters),
				}
				data, err := marshalJSON(metadata)
				if err != nil {
					return err
				}
				if err := ctx.Err(); err != nil {
					return err
				}
				return os.WriteFile(filepath.Join(input.OutputDir, "metadata.json"), data, 0o644)
			},
		},
		{
			Name:    "transcode",
			Version: "1",
			Primary: "output.bin",
			Run: func(ctx context.Context, input StageInput) error {
				source, err := os.Open(input.InputPath)
				if err != nil {
					return err
				}
				defer source.Close()
				target, err := os.OpenFile(
					filepath.Join(input.OutputDir, "output.bin"),
					os.O_CREATE|os.O_EXCL|os.O_WRONLY,
					0o644,
				)
				if err != nil {
					return err
				}
				_, copyErr := io.Copy(target, &contextReader{ctx: ctx, reader: source})
				if copyErr == nil {
					copyErr = target.Sync()
				}
				closeErr := target.Close()
				if copyErr != nil {
					return copyErr
				}
				if closeErr != nil {
					return closeErr
				}
				if err := ctx.Err(); err != nil {
					return fmt.Errorf("transcode: passthrough cancelled: %w", err)
				}
				return nil
			},
		},
	}
}
