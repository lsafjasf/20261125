package repack

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
)

// FailStage identifies a point in the transaction used for fault injection.
type FailStage string

const (
	FailAfterWrite   FailStage = "after_write"
	FailBeforeRename FailStage = "before_rename"
	FailAfterRename  FailStage = "after_rename"
	FailVerifyOutput FailStage = "verify_output"
)

// Options controls one transactional repack run.
type Options struct {
	// Inputs are source pack paths as given by the caller. Relative paths
	// are resolved against Base and may not escape it via "..".
	Inputs []string
	// Output is the destination pack path, subject to the same rules.
	Output string
	// Base anchors relative path resolution; it must be an absolute path.
	Base string

	// FailAt injects a failure at a transaction stage. Empty disables it.
	FailAt FailStage
	// CorruptTemp rewrites bytes in the temporary output before the rename.
	CorruptTemp bool
}

var langCodeRE = regexp.MustCompile(`^[a-z]{2,3}(-[A-Z][a-z]{1,7})?$`)

// Resolve turns a user-supplied path (possibly relative, with redundant
// separators or "."/".." components) into a clean absolute path. Paths that
// escape base are rejected so a relative path can never mutate into a
// location the caller did not consent to.
func Resolve(base, p string) (string, error) {
	if filepath.IsAbs(base) == false {
		return "", fmt.Errorf("base must be absolute, got %q", base)
	}
	if p == "" {
		return "", errors.New("empty path")
	}
	clean := filepath.Clean(p)
	if filepath.IsAbs(clean) {
		return clean, nil
	}
	joined := filepath.Clean(filepath.Join(base, clean))
	rel, err := filepath.Rel(base, joined)
	if err != nil {
		return "", fmt.Errorf("resolve %q: %w", p, err)
	}
	if rel == ".." || filepath.IsAbs(rel) ||
		(len(rel) >= 3 && rel[:3] == ".."+string(os.PathSeparator)) {
		return "", fmt.Errorf("path %q escapes base directory %q", p, base)
	}
	return joined, nil
}

type sourcePack struct {
	givenPath string
	absPath   string
	boxes     []Box
	lang      string
	video     []Box
	audio     []Box
	subtitles []Box
	chapters  []Box
}

// Repack performs a transactional merge of the input packs into Output.
// On any failure the output path is restored to its pre-call state and the
// error is returned; inputs are only ever opened read-only.
func Repack(opts Options) error {
	if opts.Base == "" {
		abs, err := os.Getwd()
		if err != nil {
			return fmt.Errorf("resolve base: %w", err)
		}
		opts.Base = abs
	}

	// Phase 1: path validation. Nothing is created or modified here.
	if len(opts.Inputs) == 0 {
		return errors.New("no input packs supplied")
	}
	sources := make([]*sourcePack, 0, len(opts.Inputs))
	seenPaths := map[string]string{}
	for _, given := range opts.Inputs {
		abs, err := Resolve(opts.Base, given)
		if err != nil {
			return err
		}
		if prev, dup := seenPaths[abs]; dup {
			return fmt.Errorf("duplicate input path %q (same file as %q)", given, prev)
		}
		seenPaths[abs] = given
		sources = append(sources, &sourcePack{givenPath: given, absPath: abs})
	}
	outAbs, err := Resolve(opts.Base, opts.Output)
	if err != nil {
		return err
	}
	if err := rejectSameFile(sources, outAbs); err != nil {
		return err
	}

	// Phase 2: parse and validate all inputs before touching the output.
	languages := map[string]bool{}
	for _, src := range sources {
		boxes, err := ReadPack(src.absPath)
		if err != nil {
			return err
		}
		src.boxes = boxes
		if err := classifyAndValidate(src); err != nil {
			return fmt.Errorf("input %q: %w", src.givenPath, err)
		}
		if languages[src.lang] {
			return fmt.Errorf("duplicate language code %q across inputs", src.lang)
		}
		languages[src.lang] = true
	}
	if err := crossCheck(sources); err != nil {
		return err
	}

	merged := mergeBoxes(sources)
	encoded, err := EncodePack(merged)
	if err != nil {
		return err
	}

	// Phase 3: transactional commit.
	tx, err := beginTransaction(outAbs)
	if err != nil {
		return err
	}
	commitErr := tx.commit(encoded, opts)
	if commitErr != nil {
		if rbErr := tx.rollback(); rbErr != nil {
			return fmt.Errorf("%w; rollback failed: %v", commitErr, rbErr)
		}
		return commitErr
	}
	return nil
}

func rejectSameFile(sources []*sourcePack, out string) error {
	outInfo, err := os.Stat(out)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("stat output %q: %w", out, err)
	}
	for _, src := range sources {
		inInfo, err := os.Stat(src.absPath)
		if err != nil {
			return fmt.Errorf("stat input %q: %w", src.givenPath, err)
		}
		if os.SameFile(inInfo, outInfo) {
			return fmt.Errorf("output %q must not overwrite input %q",
				src.givenPath, src.givenPath)
		}
	}
	return nil
}

func classifyAndValidate(src *sourcePack) error {
	for _, b := range src.boxes {
		switch b.Type {
		case TypeVideo:
			if b.Language != "" {
				return fmt.Errorf("video box must not carry a language, got %q", b.Language)
			}
			if b.Width == 0 || b.Height == 0 {
				return errors.New("video box missing dimensions")
			}
			src.video = append(src.video, b)
		case TypeAudio:
			if !langCodeRE.MatchString(b.Language) {
				return fmt.Errorf("invalid audio language code %q", b.Language)
			}
			if src.lang == "" {
				src.lang = b.Language
			} else if src.lang != b.Language {
				return fmt.Errorf("pack mixes audio languages %q and %q",
					src.lang, b.Language)
			}
			src.audio = append(src.audio, b)
		case TypeSubtitle:
			if !langCodeRE.MatchString(b.Language) {
				return fmt.Errorf("invalid subtitle language code %q", b.Language)
			}
			src.subtitles = append(src.subtitles, b)
		case TypeChapter:
			if b.Language != "" {
				return fmt.Errorf("chapter box must not carry a language, got %q", b.Language)
			}
			src.chapters = append(src.chapters, b)
		default:
			return fmt.Errorf("unknown box type %q", b.Type)
		}
		if b.StartUs < 0 || b.EndUs <= b.StartUs {
			return fmt.Errorf("box %s has invalid interval [%d,%d)",
				b.Type, b.StartUs, b.EndUs)
		}
	}
	if len(src.video) == 0 {
		return errors.New("missing video track")
	}
	if len(src.audio) == 0 {
		return errors.New("missing audio track")
	}
	if len(src.subtitles) == 0 {
		return errors.New("missing subtitle track")
	}
	if len(src.chapters) == 0 {
		return errors.New("missing chapters")
	}
	if src.lang == "" {
		return errors.New("could not determine language from audio track")
	}
	for _, b := range src.subtitles {
		if b.Language != src.lang {
			return fmt.Errorf("subtitle language %q does not match audio language %q",
				b.Language, src.lang)
		}
	}
	if err := checkTimeline("video", src.video); err != nil {
		return err
	}
	if err := checkTimeline("audio", src.audio); err != nil {
		return err
	}
	if err := checkTimeline("subtitle", src.subtitles); err != nil {
		return err
	}
	if err := checkTimeline("chapter", src.chapters); err != nil {
		return err
	}
	if err := checkSpan(src); err != nil {
		return err
	}
	return nil
}

func checkTimeline(name string, boxes []Box) error {
	for i := 1; i < len(boxes); i++ {
		if boxes[i].StartUs < boxes[i-1].EndUs {
			return fmt.Errorf("%s timeline not strictly ordered at index %d: "+
				"[%d,%d) follows [%d,%d)", name, i,
				boxes[i].StartUs, boxes[i].EndUs,
				boxes[i-1].StartUs, boxes[i-1].EndUs)
		}
	}
	return nil
}

func checkSpan(src *sourcePack) error {
	videoStart := src.video[0].StartUs
	videoEnd := src.video[len(src.video)-1].EndUs
	audioStart := src.audio[0].StartUs
	audioEnd := src.audio[len(src.audio)-1].EndUs
	if videoStart != audioStart {
		return fmt.Errorf("audio starts at %dus but video starts at %dus",
			audioStart, videoStart)
	}
	if videoEnd != audioEnd {
		return fmt.Errorf("audio ends at %dus but video ends at %dus",
			audioEnd, videoEnd)
	}
	// Subtitle timing is anchored to this pack's own audio timeline. This is
	// the check the old packager failed when it attached subtitle timing to a
	// different audio track.
	for _, b := range src.subtitles {
		if b.StartUs < audioStart || b.EndUs > audioEnd {
			return fmt.Errorf("subtitle cue [%d,%d) outside audio span [%d,%d)",
				b.StartUs, b.EndUs, audioStart, audioEnd)
		}
		found := false
		for _, a := range src.audio {
			if a.StartUs == b.StartUs && a.EndUs == b.EndUs {
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("subtitle cue [%d,%d) is not aligned to any "+
				"audio sample of language %q", b.StartUs, b.EndUs, src.lang)
		}
	}
	return nil
}

func crossCheck(sources []*sourcePack) error {
	ref := sources[0]
	for _, src := range sources[1:] {
		if len(src.video) != len(ref.video) {
			return fmt.Errorf("language %q has %d video frames, reference has %d",
				src.lang, len(src.video), len(ref.video))
		}
		for i, refFrame := range ref.video {
			frame := src.video[i]
			// Every language must show the same frames at the same instants:
			// a mismatch here is the "picture restarts when switching
			// language" defect.
			if frame.StartUs != refFrame.StartUs || frame.EndUs != refFrame.EndUs {
				return fmt.Errorf("video timing mismatch at frame %d: "+
					"language %q [%d,%d) vs reference [%d,%d)",
					i, src.lang, frame.StartUs, frame.EndUs,
					refFrame.StartUs, refFrame.EndUs)
			}
			if frame.Width != refFrame.Width || frame.Height != refFrame.Height ||
				!bytesEqual(frame.Payload, refFrame.Payload) {
				return fmt.Errorf("video content mismatch at frame %d for language %q",
					i, src.lang)
			}
		}
		if len(src.chapters) != len(ref.chapters) {
			return fmt.Errorf("language %q has %d chapters, reference has %d",
				src.lang, len(src.chapters), len(ref.chapters))
		}
		for i, refCh := range ref.chapters {
			ch := src.chapters[i]
			if ch.StartUs != refCh.StartUs || ch.EndUs != refCh.EndUs ||
				!bytesEqual(ch.Payload, refCh.Payload) {
				return fmt.Errorf("chapter %d mismatch in language %q", i, src.lang)
			}
		}
		if src.audio[0].StartUs != ref.audio[0].StartUs ||
			src.audio[len(src.audio)-1].EndUs != ref.audio[len(ref.audio)-1].EndUs {
			return fmt.Errorf("audio span of language %q does not match reference",
				src.lang)
		}
	}
	return nil
}

func mergeBoxes(sources []*sourcePack) []Box {
	merged := make([]Box, 0)
	// One shared video track: identical frames must not be duplicated.
	merged = append(merged, sources[0].video...)
	for _, src := range sources {
		merged = append(merged, src.audio...)
		merged = append(merged, src.subtitles...)
	}
	merged = append(merged, sources[0].chapters...)
	sort.SliceStable(merged, func(i, j int) bool {
		if merged[i].Type != merged[j].Type {
			return typeRank(merged[i].Type) < typeRank(merged[j].Type)
		}
		if merged[i].StartUs != merged[j].StartUs {
			return merged[i].StartUs < merged[j].StartUs
		}
		return merged[i].Language < merged[j].Language
	})
	return merged
}

func typeRank(t BoxType) int {
	switch t {
	case TypeVideo:
		return 0
	case TypeAudio:
		return 1
	case TypeSubtitle:
		return 2
	default:
		return 3
	}
}

func bytesEqual(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
