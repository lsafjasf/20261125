package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"tearcheck/internal/analyze"
	"tearcheck/internal/review"
)

func runReview(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		printReviewUsage(stderr)
		return exitUsage
	}
	switch args[0] {
	case "set":
		return runReviewSet(args[1:], stdout, stderr)
	case "unset":
		return runReviewUnset(args[1:], stdout, stderr)
	case "list":
		return runReviewList(args[1:], stdout, stderr)
	case "walk":
		return runReviewWalk(args[1:], stdin, stdout, stderr)
	case "help", "-h", "--help":
		printReviewUsage(stdout)
		return exitOK
	default:
		fmt.Fprintf(stderr, "unknown review command %q\n\n", args[0])
		printReviewUsage(stderr)
		return exitUsage
	}
}

func printReviewUsage(writer io.Writer) {
	fmt.Fprintln(writer, `Usage:
  tearcheck review set   --review FILE --frame ID --decision confirm_tear|clear|uncertain [--note TEXT] [--reviewer NAME] [--report FILE]
  tearcheck review unset --review FILE --frame ID
  tearcheck review list  --report FILE [--state review-needed|all|tear|uncertain|light_effect|duplicate]
  tearcheck review walk  --report FILE --review FILE [--reviewer NAME] [--all] [--notes]

Review decisions are applied by the next analyze run:
  tearcheck analyze --input DIR --review FILE --out report.json`)
}

func runReviewSet(args []string, stdout, stderr io.Writer) int {
	set := flag.NewFlagSet("review set", flag.ContinueOnError)
	set.SetOutput(stderr)
	path := set.String("review", "", "review JSON file to create or update")
	frameID := set.String("frame", "", "frame id to correct")
	decision := set.String("decision", "", "confirm_tear, clear, or uncertain")
	note := set.String("note", "", "reviewer note")
	reviewer := set.String("reviewer", "", "reviewer name")
	reportPath := set.String("report", "", "optional report used to validate the frame id")
	if err := set.Parse(args); err != nil {
		return exitUsage
	}
	if *path == "" || *frameID == "" || *decision == "" {
		fmt.Fprintln(stderr, "review set: --review, --frame, and --decision are required")
		return exitUsage
	}
	file, err := loadReviewFile(*path, true)
	if err != nil {
		fmt.Fprintf(stderr, "review set: %v\n", err)
		return exitError
	}
	if *reportPath != "" {
		if err := validateFrameInReport(*reportPath, *frameID); err != nil {
			fmt.Fprintf(stderr, "review set: %v\n", err)
			return exitError
		}
	}
	updated, err := review.Set(file, review.Entry{
		FrameID:  *frameID,
		Decision: *decision,
		Note:     *note,
		Reviewer: *reviewer,
	})
	if err != nil {
		fmt.Fprintf(stderr, "review set: %v\n", err)
		return exitError
	}
	if err := review.WriteAtomic(*path, updated); err != nil {
		fmt.Fprintf(stderr, "review set: %v\n", err)
		return exitError
	}
	fmt.Fprintf(stdout, "frame=%s decision=%s\n", strings.TrimSpace(*frameID), canonicalDecisionForOutput(updated, *frameID))
	return exitOK
}

func runReviewUnset(args []string, stdout, stderr io.Writer) int {
	set := flag.NewFlagSet("review unset", flag.ContinueOnError)
	set.SetOutput(stderr)
	path := set.String("review", "", "review JSON file to update")
	frameID := set.String("frame", "", "frame id whose correction should be removed")
	if err := set.Parse(args); err != nil {
		return exitUsage
	}
	if *path == "" || *frameID == "" {
		fmt.Fprintln(stderr, "review unset: --review and --frame are required")
		return exitUsage
	}
	file, err := loadReviewFile(*path, false)
	if err != nil {
		fmt.Fprintf(stderr, "review unset: %v\n", err)
		return exitError
	}
	updated, err := review.Remove(file, *frameID)
	if err != nil {
		fmt.Fprintf(stderr, "review unset: %v\n", err)
		return exitError
	}
	if err := review.WriteAtomic(*path, updated); err != nil {
		fmt.Fprintf(stderr, "review unset: %v\n", err)
		return exitError
	}
	fmt.Fprintf(stdout, "frame=%s decision=removed\n", strings.TrimSpace(*frameID))
	return exitOK
}

func runReviewList(args []string, stdout, stderr io.Writer) int {
	set := flag.NewFlagSet("review list", flag.ContinueOnError)
	set.SetOutput(stderr)
	reportPath := set.String("report", "", "report JSON to inspect")
	state := set.String("state", "review-needed", "all, review-needed, tear, uncertain, light_effect, duplicate, not_tear, reviewed")
	if err := set.Parse(args); err != nil {
		return exitUsage
	}
	if *reportPath == "" {
		fmt.Fprintln(stderr, "review list: --report is required")
		return exitUsage
	}
	switch strings.ToLower(strings.TrimSpace(*state)) {
	case "all", "review-needed", "reviewed", "tear", "uncertain", "light_effect", "light", "duplicate", "not_tear":
	default:
		fmt.Fprintf(stderr, "review list: unknown state %q\n", *state)
		return exitUsage
	}
	report, err := loadReport(*reportPath)
	if err != nil {
		fmt.Fprintf(stderr, "review list: %v\n", err)
		return exitError
	}
	fmt.Fprintln(stdout, "index\tframe_id\ttimecode\tpts_us\tmachine\tfinal\twidth_px\tcontinuity\tevent_id\tduplicate_of\treview")
	for _, frame := range report.Frames {
		if !matchesReviewState(frame, *state) {
			continue
		}
		eventID := frame.Machine.EventID
		if eventID == "" {
			eventID = "-"
		}
		reviewText := "-"
		if frame.Review != nil {
			reviewText = frame.Review.Decision
			if frame.Review.Note != "" {
				reviewText += ": " + frame.Review.Note
			}
		}
		duplicateOf := frame.DuplicateOf
		if duplicateOf == "" {
			duplicateOf = "-"
		}
		fmt.Fprintf(stdout, "%d\t%s\t%s\t%d\t%s\t%s\t%d\t%s\t%s\t%s\t%s\n",
			frame.Index,
			frame.ID,
			frame.Timecode,
			frame.PTSUS,
			frame.Machine.Classification,
			frame.Final,
			frame.Machine.WidthPX,
			frame.Machine.Continuity,
			eventID,
			duplicateOf,
			singleLine(reviewText))
	}
	return exitOK
}

func runReviewWalk(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	set := flag.NewFlagSet("review walk", flag.ContinueOnError)
	set.SetOutput(stderr)
	reportPath := set.String("report", "", "report JSON to review")
	reviewPath := set.String("review", "", "review JSON file to create or update")
	reviewer := set.String("reviewer", "", "reviewer name stored with each decision")
	all := set.Bool("all", false, "walk every frame instead of only ambiguous frames")
	askNotes := set.Bool("notes", false, "prompt for an optional note after each decision")
	if err := set.Parse(args); err != nil {
		return exitUsage
	}
	if *reportPath == "" || *reviewPath == "" {
		fmt.Fprintln(stderr, "review walk: --report and --review are required")
		return exitUsage
	}
	report, err := loadReport(*reportPath)
	if err != nil {
		fmt.Fprintf(stderr, "review walk: %v\n", err)
		return exitError
	}
	file, err := loadReviewFile(*reviewPath, true)
	if err != nil {
		fmt.Fprintf(stderr, "review walk: %v\n", err)
		return exitError
	}
	reader := bufio.NewReader(stdin)
	total := 0
	for _, frame := range report.Frames {
		if *all || needsReview(frame) {
			total++
		}
	}
	if total == 0 {
		fmt.Fprintln(stdout, "no frames need review")
		return exitOK
	}

	position := 0
	changed := 0
frameLoop:
	for _, frame := range report.Frames {
		if !*all && !needsReview(frame) {
			continue
		}
		position++
		eventID := frame.Machine.EventID
		if eventID == "" {
			eventID = "-"
		}
		fmt.Fprintf(stdout, "\n[%d/%d] %s %s machine=%s final=%s width=%d continuity=%s event=%s file=%s\n",
			position, total, frame.ID, frame.Timecode, frame.Machine.Classification,
			frame.Final, frame.Machine.WidthPX, frame.Machine.Continuity, eventID, frame.File)
		if len(frame.Machine.Seams) > 0 {
			seam := frame.Machine.Seams[0]
			fmt.Fprintf(stdout, "  seam row=%d offset=%d match=%dbp gain=%dbp bias=%dbp structured=%d\n",
				seam.Row, seam.OffsetPX, seam.MatchScoreBP, seam.ShiftGainBP, seam.BiasRatioBP, seam.StructuredDiff)
		}
		for {
			fmt.Fprint(stdout, "  [c]onfirm tear  [x] clear  [u]ncertain  [s]kip  [q]uit: ")
			line, readErr := reader.ReadString('\n')
			if readErr != nil && len(strings.TrimSpace(line)) == 0 {
				fmt.Fprintln(stdout)
				if err := writeWalkReview(*reviewPath, file, stderr); err != nil {
					return exitError
				}
				fmt.Fprintf(stdout, "wrote %d decision(s) to %s\n", changed, *reviewPath)
				return exitOK
			}
			answer := strings.ToLower(strings.TrimSpace(line))
			if answer == "" {
				continue
			}
			switch answer[0] {
			case 'c', 'x', 'u':
				decision := review.DecisionConfirmTear
				if answer[0] == 'x' {
					decision = review.DecisionClear
				} else if answer[0] == 'u' {
					decision = review.DecisionUncertain
				}
				entry := review.Entry{FrameID: frame.ID, Decision: decision}
				if existing, ok := findReviewEntry(file, frame.ID); ok {
					entry.Note = existing.Note
					entry.Reviewer = existing.Reviewer
				}
				if *reviewer != "" {
					entry.Reviewer = *reviewer
				}
				if *askNotes {
					fmt.Fprint(stdout, "  note (enter to keep): ")
					note, _ := reader.ReadString('\n')
					note = strings.TrimSpace(note)
					if note != "" {
						entry.Note = note
					}
				}
				file, err = review.Set(file, entry)
				if err != nil {
					fmt.Fprintf(stderr, "review walk: %v\n", err)
					return exitError
				}
				changed++
				continue frameLoop
			case 's':
				continue frameLoop
			case 'q':
				if err := writeWalkReview(*reviewPath, file, stderr); err != nil {
					return exitError
				}
				fmt.Fprintf(stdout, "wrote %d decision(s) to %s\n", changed, *reviewPath)
				return exitOK
			default:
				fmt.Fprintln(stdout, "  unknown choice")
			}
		}
	}
	if err := writeWalkReview(*reviewPath, file, stderr); err != nil {
		return exitError
	}
	fmt.Fprintf(stdout, "wrote %d decision(s) to %s\n", changed, *reviewPath)
	return exitOK
}

func writeWalkReview(path string, file *review.File, stderr io.Writer) error {
	if err := review.WriteAtomic(path, file); err != nil {
		fmt.Fprintf(stderr, "review walk: %v\n", err)
		return err
	}
	return nil
}

func loadReviewFile(path string, allowMissing bool) (*review.File, error) {
	file, _, err := review.Load(path)
	if err == nil {
		return file, nil
	}
	if allowMissing && errors.Is(err, os.ErrNotExist) {
		return review.NewFile(), nil
	}
	return nil, err
}

func loadReport(path string) (*analyze.Report, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var report analyze.Report
	if err := json.Unmarshal(raw, &report); err != nil {
		return nil, fmt.Errorf("parse report: %w", err)
	}
	if report.Schema != analyze.SchemaVersion {
		return nil, fmt.Errorf("unsupported report schema %q", report.Schema)
	}
	return &report, nil
}

func validateFrameInReport(path, frameID string) error {
	report, err := loadReport(path)
	if err != nil {
		return err
	}
	for _, frame := range report.Frames {
		if frame.ID == frameID {
			return nil
		}
	}
	return fmt.Errorf("frame %q is not present in report", frameID)
}

func canonicalDecisionForOutput(file *review.File, frameID string) string {
	if entry, ok := findReviewEntry(file, frameID); ok {
		return entry.Decision
	}
	return ""
}

func findReviewEntry(file *review.File, frameID string) (review.Entry, bool) {
	for _, entry := range file.Entries {
		if entry.FrameID == frameID {
			return entry, true
		}
	}
	return review.Entry{}, false
}

func needsReview(frame analyze.FrameInfo) bool {
	if frame.Review != nil {
		return frame.Final == "uncertain"
	}
	switch frame.Final {
	case "tear", "uncertain":
		return true
	}
	return frame.Machine.Classification == "light_effect"
}

func matchesReviewState(frame analyze.FrameInfo, state string) bool {
	switch strings.ToLower(strings.TrimSpace(state)) {
	case "all":
		return true
	case "review-needed":
		return needsReview(frame)
	case "reviewed":
		return frame.Review != nil
	case "tear":
		return frame.Final == "tear"
	case "uncertain":
		return frame.Final == "uncertain"
	case "light_effect", "light":
		return frame.Machine.Classification == "light_effect"
	case "duplicate":
		return frame.Machine.Classification == "duplicate"
	case "not_tear":
		return frame.Final == "not_tear"
	default:
		return false
	}
}

func singleLine(value string) string {
	value = strings.ReplaceAll(value, "\t", " ")
	value = strings.ReplaceAll(value, "\r", " ")
	value = strings.ReplaceAll(value, "\n", " ")
	return value
}
