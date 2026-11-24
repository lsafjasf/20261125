// auditdemo is a standard-library-only demonstration of the multi-source
// auditor. Real deployments replace the static observations below with
// implementations of auditor.Provider (directory walker, container parser,
// bitstream decoder, sidecar reader).
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"time"

	"multisourceaudit/auditor"
)

var collectedAt = time.Date(2026, 11, 25, 8, 0, 0, 0, time.UTC)

func ob(mediaID string, source auditor.Source, field auditor.Field, raw, detail string) auditor.Observation {
	return auditor.Observation{
		MediaID:    mediaID,
		Source:     source,
		Field:      field,
		RawValue:   raw,
		ObservedAt: collectedAt,
		Detail:     detail,
	}
}

// demoObservations returns representative four-source evidence for three
// media items with deliberately different outcomes.
func demoObservations() []auditor.Observation {
	return []auditor.Observation{
		// Item A: all trustworthy sources agree; file system agrees after
		// normalisation -> automatic high confidence.
		ob("movie-A", auditor.SourceBitstream, auditor.FieldLanguage, "en", "audio track #1"),
		ob("movie-A", auditor.SourceContainer, auditor.FieldLanguage, "eng", "mvhd/hdlr tags"),
		ob("movie-A", auditor.SourceSidecar, auditor.FieldLanguage, "en-US", "movie-A.nfo"),
		ob("movie-A", auditor.SourceFilesystem, auditor.FieldLanguage, "english", "directory name"),
		ob("movie-A", auditor.SourceBitstream, auditor.FieldDuration, "01:42:03.500", "frame count / frame rate"),
		ob("movie-A", auditor.SourceContainer, auditor.FieldDuration, "6123.5s", "mvhd duration"),
		ob("movie-A", auditor.SourceSidecar, auditor.FieldDuration, "6123.5", "movie-A.nfo"),
		ob("movie-A", auditor.SourceFilesystem, auditor.FieldDuration, "6123.5", "stat mtime heuristic"),

		// Item B: bitstream and container disagree -> manual review. The
		// file-system value is shown in provenance but never used.
		ob("movie-B", auditor.SourceBitstream, auditor.FieldLanguage, "ja", "audio track #1"),
		ob("movie-B", auditor.SourceContainer, auditor.FieldLanguage, "en", "container tags"),
		ob("movie-B", auditor.SourceSidecar, auditor.FieldLanguage, "jpn", "movie-B.json"),
		ob("movie-B", auditor.SourceFilesystem, auditor.FieldLanguage, "fr", "file name guess"),
		ob("movie-B", auditor.SourceBitstream, auditor.FieldDuration, "5400s", "decoded frames"),
		ob("movie-B", auditor.SourceContainer, auditor.FieldDuration, "5398.2s", "header duration"),
		ob("movie-B", auditor.SourceFilesystem, auditor.FieldDuration, "5400", "file size heuristic"),

		// Item C: only the file system has any claim -> undetermined, with
		// no fallback value emitted.
		ob("movie-C", auditor.SourceFilesystem, auditor.FieldLanguage, "de", "directory name"),
		ob("movie-C", auditor.SourceFilesystem, auditor.FieldDuration, "3600", "file size heuristic"),
	}
}

func main() {
	textMode := flag.Bool("text", false, "emit a compact text summary instead of JSON")
	onlyMedia := flag.String("media", "", "limit the report to one media id")
	flag.Parse()

	observations := demoObservations()
	mediaIDs := []string{"movie-A", "movie-B", "movie-C"}

	var reports []auditor.Report
	for _, mediaID := range mediaIDs {
		if *onlyMedia != "" && *onlyMedia != mediaID {
			continue
		}
		var scoped []auditor.Observation
		for _, observation := range observations {
			if observation.MediaID == mediaID {
				scoped = append(scoped, observation)
			}
		}
		reports = append(reports, auditor.Review(mediaID, scoped, auditor.DefaultPolicy()))
	}

	if *textMode {
		for _, report := range reports {
			writeTextReport(os.Stdout, report)
		}
		return
	}
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	if len(reports) == 1 {
		must(encoder.Encode(reports[0]))
		return
	}
	must(encoder.Encode(reports))
}

func must(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, "auditdemo:", err)
		os.Exit(1)
	}
}
