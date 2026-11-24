package anc

// resolveContinuations gives every truncated candidate a chance to be
// completed from the beginning of the immediately following input line(s).
// The primary classification stays ClassTruncated: a standards-compliant ANC
// packet never spans video lines, so the condition itself is the anomaly.
func resolveContinuations(cands []*candidate, nls []*normLine, cfg Options) {
	for _, c := range cands {
		if c.pkt.Classification != ClassTruncated || c.missingUnits <= 0 {
			continue
		}
		missing := c.missingUnits
		taken := 0
		var gathered []Word
		var contRaw []byte
		var loc *Location
		nextADF := false

		for step := 1; step <= cfg.ContinuationLines && missing > 0; step++ {
			li := c.lineIndex + step
			if li >= len(nls) {
				break
			}
			nl2 := nls[li]
			if len(nl2.units) >= 3 && nl2.adfAt(0) {
				nextADF = true
				break
			}
			if len(nl2.units) == 0 {
				continue
			}
			take := minInt(missing, len(nl2.units))
			if loc == nil {
				u0 := nl2.units[0]
				loc = &Location{
					Field:        nl2.line.Field,
					Line:         nl2.line.Number,
					ByteOffset:   u0.start,
					WordOffset:   0,
					StreamOffset: nl2.line.StreamOffset + u0.start,
				}
			}
			lastU := nl2.units[take-1]
			rawEnd := minInt(lastU.end, len(nl2.raw))
			contRaw = append(contRaw, nl2.raw[nl2.units[0].start:rawEnd]...)
			for k := 0; k < take; k++ {
				gathered = append(gathered, nl2.units[k].w)
			}
			taken += take
			missing -= take
		}

		cont := &Continuation{
			MissingUnits:  c.missingUnits,
			UnitsTaken:    taken,
			NextStartsADF: nextADF,
		}
		if loc != nil {
			cont.Location = *loc
			cont.RawHex = hexEncode(contRaw)
		}

		if missing > 0 || len(gathered) != c.missingUnits {
			if loc != nil || nextADF {
				c.pkt.Continuation = cont
			}
			continue
		}

		// Exactly the missing number of units was gathered: the final one is
		// the candidate CS word, the rest completes the UDW sequence.
		reassembled := true
		cont.Reassembled = reassembled
		fullUDW := append(append([]Word{}, c.udwWords...), gathered[:len(gathered)-1]...)
		csW := gathered[len(gathered)-1]

		payload := make([]byte, len(fullUDW))
		udwInts := make([]int, len(fullUDW))
		for i, w := range fullUDW {
			payload[i] = w.Data()
			udwInts[i] = int(w)
		}
		c.pkt.PayloadHex = hexEncode(payload)
		c.pkt.PayloadASCII = asciiPreview(payload)
		c.pkt.UDW = udwInts

		verifyChecksum(c.pkt, nls[c.lineIndex].enc, c.headerWords, fullUDW, csW)
		if checksumHealthy(c.pkt) {
			cont.ChecksumOK = true
			c.pkt.Labels = appendUnique(c.pkt.Labels, LabelReassembledValid)
			c.pkt.Reasons = append(c.pkt.Reasons, "reassembled_across_lines: continuation checksum verifies")
		} else {
			c.pkt.Reasons = append(c.pkt.Reasons, "continuation_present_but_checksum_invalid")
		}
		c.pkt.Labels = appendUnique(c.pkt.Labels, LabelCrossLine)
		c.coreKey = contentKey(c.pkt.DID, c.pkt.SDIDOrDBN, c.pkt.DC, fullUDW)
		if loc != nil || nextADF {
			c.pkt.Continuation = cont
		}
	}
}
