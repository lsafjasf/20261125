package analyze

import "fmt"

// suggest produces manual splice recommendations from the detected
// issues and the boundaries between playable ranges.
func suggest(rep *Report) []string {
	var out []string

	// Boundary advice between consecutive playable ranges.
	for i := 1; i < len(rep.Ranges); i++ {
		prev := rep.Ranges[i-1].Frames
		cur := rep.Ranges[i].Frames
		if len(prev) == 0 || len(cur) == 0 {
			continue
		}
		a := prev[len(prev)-1]
		b := cur[0]
		switch {
		case a.Header.Width != b.Header.Width || a.Header.Height != b.Header.Height:
			out = append(out, fmt.Sprintf(
				"splice %d->%d: resolution changes %dx%d -> %dx%d at %s; conform both sides to a common raster (or keep as separate timelines) before joining",
				i, i+1, a.Header.Width, a.Header.Height, b.Header.Width, b.Header.Height, short(b.Header.Path)))
		case b.Key-a.Key > 1:
			out = append(out, fmt.Sprintf(
				"splice %d->%d: %d frame(s) missing (keys %d..%d) between %s and %s; request re-delivery, or bridge with slug/black and flag the join for review",
				i, i+1, b.Key-a.Key-1, a.Key+1, b.Key-1, short(a.Header.Path), short(b.Header.Path)))
		case b.Key <= a.Key && a.Reel != b.Reel:
			out = append(out, fmt.Sprintf(
				"splice %d->%d: ordering key rolls back %d -> %d at reel change %s -> %s; expected after a reel change — confirm reel order before joining %s | %s",
				i, i+1, a.Key, b.Key, a.Reel, b.Reel, short(a.Header.Path), short(b.Header.Path)))
		default:
			out = append(out, fmt.Sprintf(
				"splice %d->%d: frames are contiguous but were split across reels (%s -> %s); join directly at %s | %s",
				i, i+1, a.Reel, b.Reel, short(a.Header.Path), short(b.Header.Path)))
		}
	}

	// Cross-reel timecode rollback check.
	for i := 1; i < len(rep.Frames); i++ {
		a, b := rep.Frames[i-1], rep.Frames[i]
		if a.Reel == b.Reel || !a.Header.HasTimeCode || !b.Header.HasTimeCode {
			continue
		}
		ta := a.Header.TimeCode.AbsFrames(rep.FPS)
		tb := b.Header.TimeCode.AbsFrames(rep.FPS)
		if tb <= ta {
			out = append(out, fmt.Sprintf(
				"reel boundary %s -> %s: timecode rolls back %s -> %s (expected after a reel change); confirm reel order and re-strip a continuous timeline TC starting at %s",
				a.Reel, b.Reel, a.Header.TimeCode, b.Header.TimeCode, a.Header.TimeCode))
		}
	}

	// Issue-driven advice.
	dups, gaps, resChg, missing := 0, 0, false, 0
	for _, is := range rep.Issues {
		switch is.Kind {
		case "duplicate":
			dups++
		case "gap":
			gaps++
		case "resolution-change":
			resChg = true
		case "manifest-missing":
			missing++
		}
	}
	if dups > 0 {
		out = append(out, fmt.Sprintf("%d duplicate frame(s) detected; keep the md5-verified copy and drop the rest before conform", dups))
	}
	if gaps > 0 {
		out = append(out, fmt.Sprintf("%d gap(s) detected; hold on the last good frame or insert slug, and chase the missing reels with the vendor", gaps))
	}
	if resChg {
		out = append(out, "mixed resolutions in one delivery; split the conform per raster or standardize with a resize pass")
	}
	if missing > 0 {
		out = append(out, fmt.Sprintf("%d manifest entr(ies) have no matching file on disk; verify the transfer is complete", missing))
	}
	if len(rep.Ranges) == 1 && dups == 0 && gaps == 0 {
		out = append(out, "sequence is continuous; no manual splicing required")
	}
	return out
}
