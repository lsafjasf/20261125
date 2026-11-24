// Package confirm implements explicit user confirmation before successful
// artifacts are overwritten.
package confirm

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"strings"
)

// ErrConfirmationUnavailable means no answer could be read (for example a
// non-interactive stdin without --yes). Callers must treat this as "no".
var ErrConfirmationUnavailable = errors.New("overwrite requires explicit confirmation")

// Target describes one successful artifact that --force would replace.
type Target struct {
	JobID    string
	Artifact string
	SHA256   string
}

// Prompter asks one batch-wide overwrite question.
type Prompter struct {
	In        io.Reader
	Out       io.Writer
	AssumeYes bool
}

// ConfirmOverwrite lists every protected artifact and requires the user to
// type the full word "yes". AssumeYes (--yes) is the scriptable form of
// explicit confirmation.
func (p *Prompter) ConfirmOverwrite(targets []Target) (bool, error) {
	if len(targets) == 0 {
		return true, nil
	}
	if p.AssumeYes {
		return true, nil
	}

	var out io.Writer = p.Out
	if out == nil {
		out = io.Discard
	}
	fmt.Fprintf(out, "即将覆盖 %d 个已成功的产物:\n", len(targets))
	for _, t := range targets {
		digest := t.SHA256
		if len(digest) > 12 {
			digest = digest[:12]
		}
		fmt.Fprintf(out, "  - 任务 %s -> %s (sha256:%s…)\n", t.JobID, t.Artifact, digest)
	}
	fmt.Fprint(out, "这些产物有有效完成标记，覆盖后不可恢复。请输入 yes 确认，其它输入取消: ")

	if p.In == nil {
		return false, ErrConfirmationUnavailable
	}
	reader := bufio.NewReader(p.In)
	line, err := reader.ReadString('\n')
	if errors.Is(err, io.EOF) && line == "" {
		fmt.Fprintln(out)
		return false, ErrConfirmationUnavailable
	}
	if err != nil && !errors.Is(err, io.EOF) {
		return false, fmt.Errorf("%w: %v", ErrConfirmationUnavailable, err)
	}
	answer := strings.TrimSpace(strings.ToLower(line))
	if answer == "yes" {
		return true, nil
	}
	fmt.Fprintln(out, "已取消：未输入 yes，所有成功产物保持不变。")
	return false, nil
}
