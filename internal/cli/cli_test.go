package cli

import (
	"bytes"
	"strings"
	"testing"

	"ffv1compare/internal/testutil"
)

func TestRunJSON(t *testing.T) {
	var out, errOut bytes.Buffer
	code := Run([]string{
		"-json",
		"-a", testutil.DataPath("machine-a.mkv"),
		"-b", testutil.DataPath("machine-a-copy.mkv"),
		"-hash-a", testutil.DataPath("hashes-a.framemd5"),
		"-hash-b", testutil.DataPath("hashes-a.framemd5"),
	}, &out, &errOut)
	if code != 0 {
		t.Fatalf("exit=%d stderr=%s", code, errOut.String())
	}
	if !strings.Contains(out.String(), `"verdict": "no_difference"`) {
		t.Fatalf("unexpected JSON: %s", out.String())
	}
}

func TestRunUsageError(t *testing.T) {
	var out, errOut bytes.Buffer
	if code := Run(nil, &out, &errOut); code != 2 {
		t.Fatalf("exit=%d, want 2", code)
	}
	if !strings.Contains(errOut.String(), "Usage:") {
		t.Fatalf("missing usage: %s", errOut.String())
	}
}
