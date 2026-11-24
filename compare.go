package main

import (
	"fmt"
)

// CompareFiles opens two files, builds manifests, and compares every layer.
func CompareFiles(originalPath, copyPath string, opts Options) (*Report, error) {
	a, err := OpenArchive(originalPath, opts)
	if err != nil {
		return nil, fmt.Errorf("open original: %w", err)
	}
	defer a.Close()

	b, err := OpenArchive(copyPath, opts)
	if err != nil {
		return nil, fmt.Errorf("open copy: %w", err)
	}
	defer b.Close()

	return CompareArchives(a, b)
}

// CompareArchives compares layered hashes and locates semantic changes.
func CompareArchives(a, b *Archive) (*Report, error) {
	rep := &Report{
		Schema:       SchemaVersion,
		OriginalFile: a.Path,
		CopyFile:     b.Path,
		Format:       a.Manifest.Format,
	}
	rep.Warnings = append(rep.Warnings, prefixWarnings("original", a.Manifest.Warnings)...)
	rep.Warnings = append(rep.Warnings, prefixWarnings("copy", b.Manifest.Warnings)...)

	if a.Manifest.Format != b.Manifest.Format {
		rep.Format = "mixed"
		rep.Warnings = append(rep.Warnings, fmt.Sprintf("format mismatch: %s vs %s", a.Manifest.Format, b.Manifest.Format))
	}

	if a.Manifest.SHA256 == b.Manifest.SHA256 && a.Size == b.Size {
		rep.WholeFileIdentical = true
		rep.MediaIdentical = true
		rep.ContainerIdentical = true
		return rep, nil
	}

	rep.ContainerIdentical = false
	if err := compareRawLayer(a, b, rep); err != nil {
		return nil, err
	}
	compareBoxes(a.Manifest, b.Manifest, rep)
	compareChunks(a.Manifest, b.Manifest, rep)
	compareTrackSamples(a, b, rep)
	compareTrackFormats(a.Manifest, b.Manifest, rep)
	compareAudioIntervals(a, b, rep)

	for _, change := range rep.Changes {
		if change.Severity == "media" {
			rep.MediaIdentical = false
			break
		}
	}
	if len(rep.Changes) == 0 && (rep.Format == FormatMP4 || rep.Format == FormatWAV) {
		rep.MediaIdentical = true
	}
	if rep.Format == FormatOther && !rep.WholeFileIdentical {
		rep.MediaIdentical = false
		rep.Warnings = append(rep.Warnings, "unsupported or unrecognized media container; byte differences cannot be mapped to picture or sound")
	}
	rep.NormalRearrangement = len(rep.Rearrangements) > 0
	rep.EarliestMediaChange = selectEarliestMediaChange(rep.Changes)
	return rep, nil
}

func prefixWarnings(prefix string, warnings []string) []string {
	out := make([]string, 0, len(warnings))
	for _, w := range warnings {
		out = append(out, prefix+": "+w)
	}
	return out
}

func compareRawLayer(a, b *Archive, rep *Report) error {
	diff, err := earliestDifferenceFromBlocks(a, b)
	if err != nil {
		return err
	}
	if diff == nil {
		return nil
	}

	change := Change{
		Layer:          LayerByte,
		Kind:           "earliest_raw_byte",
		Severity:       "container",
		Message:        "first physical byte difference",
		OriginalOffset: diff.OriginalOffset,
		CopyOffset:     diff.CopyOffset,
		RelativeOffset: diff.OriginalOffset,
		OriginalEnd:    a.Size,
		CopyEnd:        b.Size,
	}
	if diff.LengthMismatch {
		change.Kind = "length_or_prefix_difference"
		change.Message = fmt.Sprintf("common prefix ends at byte %d; lengths %d and %d", diff.Relative, a.Size, b.Size)
	}
	if block := containingBlock(a.Manifest.Blocks, diff.OriginalOffset); block != nil {
		change.BlockIndex = block.Index
	}
	rep.EarliestByte = &change
	rep.addContainer(change)

	for i := 0; i < len(a.Manifest.Blocks) && i < len(b.Manifest.Blocks); i++ {
		if a.Manifest.Blocks[i].SHA256 == b.Manifest.Blocks[i].SHA256 {
			continue
		}
		rep.addContainer(Change{
			Layer:          LayerRawBlock,
			Kind:           "block_hash_mismatch",
			BlockIndex:     i,
			OriginalOffset: a.Manifest.Blocks[i].Start,
			CopyOffset:     b.Manifest.Blocks[i].Start,
			OriginalEnd:    a.Manifest.Blocks[i].End,
			CopyEnd:        b.Manifest.Blocks[i].End,
			OriginalHash:   a.Manifest.Blocks[i].SHA256,
			CopyHash:       b.Manifest.Blocks[i].SHA256,
		})
		break
	}
	return nil
}

func earliestDifferenceFromBlocks(a, b *Archive) (*byteDifference, error) {
	blocksA := a.Manifest.Blocks
	blocksB := b.Manifest.Blocks
	common := len(blocksA)
	if len(blocksB) < common {
		common = len(blocksB)
	}

	for i := 0; i < common; i++ {
		if blocksA[i].SHA256 == blocksB[i].SHA256 {
			continue
		}
		diff, err := firstRangeDifference(
			a.Reader, blocksA[i].Start, blocksA[i].End,
			b.Reader, blocksB[i].Start, blocksB[i].End,
		)
		if err != nil || diff != nil {
			return diff, err
		}
	}

	if a.Size != b.Size {
		limit := a.Size
		if b.Size < limit {
			limit = b.Size
		}
		return &byteDifference{
			Relative:       0,
			OriginalOffset: limit,
			CopyOffset:     limit,
			OriginalLength: a.Size,
			CopyLength:     b.Size,
			LengthMismatch: true,
		}, nil
	}
	return firstFileDifference(a.Reader, b.Reader, a.Size, b.Size)
}

func containingBlock(blocks []RangeHash, offset int64) *RangeHash {
	for i := range blocks {
		if offset >= blocks[i].Start && offset < blocks[i].End {
			return &blocks[i]
		}
	}
	if len(blocks) > 0 && offset >= blocks[len(blocks)-1].End {
		return &blocks[len(blocks)-1]
	}
	return nil
}

func compareBoxes(a, b *Manifest, rep *Report) {
	bByPath := make(map[string]BoxHash, len(b.Boxes))
	for _, box := range b.Boxes {
		bByPath[box.Path] = box
	}
	usedB := make(map[string]bool)

	for _, before := range a.Boxes {
		after, ok := bByPath[before.Path]
		if !ok {
			change := Change{Layer: LayerBox, BoxPath: before.Path, Kind: "box_missing", OriginalOffset: before.Start, OriginalEnd: before.End, OriginalHash: before.SHA256}
			if before.Ignorable {
				rep.addIgnored(change)
			} else {
				rep.addContainer(change)
			}
			continue
		}
		usedB[before.Path] = true
		if before.SHA256 == after.SHA256 {
			continue
		}
		change := Change{
			Layer:          LayerBox,
			BoxPath:        before.Path,
			Kind:           "box_hash_mismatch",
			OriginalOffset: before.Start,
			CopyOffset:     after.Start,
			OriginalEnd:    before.End,
			CopyEnd:        after.End,
			OriginalHash:   before.SHA256,
			CopyHash:       after.SHA256,
		}
		if before.Ignorable || after.Ignorable {
			change.Kind = "ignorable_box_changed"
			rep.addIgnored(change)
		} else {
			rep.addContainer(change)
		}
	}

	for _, after := range b.Boxes {
		if usedB[after.Path] {
			continue
		}
		change := Change{Layer: LayerBox, BoxPath: after.Path, Kind: "box_added", CopyOffset: after.Start, CopyEnd: after.End, CopyHash: after.SHA256}
		if after.Ignorable {
			rep.addIgnored(change)
		} else {
			rep.addContainer(change)
		}
	}
}

func compareChunks(a, b *Manifest, rep *Report) {
	bByKey := make(map[string]RangeHash, len(b.Chunks))
	hashLocations := make(map[string]string, len(b.Chunks))
	for _, chunk := range b.Chunks {
		bByKey[chunk.Key] = chunk
		if _, exists := hashLocations[chunk.SHA256]; !exists {
			hashLocations[chunk.SHA256] = chunk.Key
		}
	}

	for _, before := range a.Chunks {
		after, ok := bByKey[before.Key]
		if !ok || before.SHA256 != after.SHA256 {
			kind := "chunk_hash_mismatch"
			message := "container chunk layout or bytes changed"
			if movedKey, exists := hashLocations[before.SHA256]; exists && movedKey != before.Key {
				kind = "chunk_relocated"
				message = "identical chunk moved or rechunked under another key"
			}
			rep.addContainer(Change{
				Layer:          LayerMP4Chunk,
				Kind:           kind,
				Message:        message,
				OriginalOffset: before.Start,
				CopyOffset:     after.Start,
				OriginalEnd:    before.End,
				CopyEnd:        after.End,
				OriginalHash:   before.SHA256,
				CopyHash:       after.SHA256,
				ChunkIndex:     before.Index,
			})
		}
	}
}

func selectEarliestMediaChange(changes []Change) *Change {
	var best *Change
	for i := range changes {
		c := changes[i]
		if c.Severity != "media" {
			continue
		}
		if best == nil || changeEarlier(c, *best) {
			cp := c
			best = &cp
		}
	}
	return best
}

func changeEarlier(a, b Change) bool {
	if a.MetadataOnly != b.MetadataOnly {
		return !a.MetadataOnly
	}
	rankA, rankB := layerRank(a.Layer), layerRank(b.Layer)
	if a.OriginalOffset != b.OriginalOffset {
		return a.OriginalOffset < b.OriginalOffset
	}
	if rankA != rankB {
		return rankA < rankB
	}
	return a.RelativeOffset < b.RelativeOffset
}

func layerRank(layer string) int {
	switch layer {
	case LayerSample:
		return 0
	case LayerAudioInterval:
		return 1
	default:
		return 2
	}
}
