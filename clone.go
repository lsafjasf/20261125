package otioflat

func cloneTime(t *RationalTime) *RationalTime {
	if t == nil {
		return nil
	}
	copied := *t
	return &copied
}

func cloneRange(r *TimeRange) *TimeRange {
	if r == nil {
		return nil
	}
	return &TimeRange{
		StartTime: *cloneTime(&r.StartTime),
		Duration:  *cloneTime(&r.Duration),
	}
}

func cloneMetadata(m Metadata) Metadata {
	if m == nil {
		return nil
	}
	out := make(Metadata, len(m))
	for key, value := range m {
		out[key] = value
	}
	return out
}

func cloneMedia(ref *MediaReference) *MediaReference {
	if ref == nil {
		return nil
	}
	copied := *ref
	copied.AvailableRange = cloneRange(ref.AvailableRange)
	copied.Metadata = cloneMetadata(ref.Metadata)
	return &copied
}

func cloneStamps(path []Stamp) []Stamp {
	if path == nil {
		return nil
	}
	out := make([]Stamp, len(path))
	for i, stamp := range path {
		out[i] = cloneStamp(stamp)
	}
	return out
}

func cloneStamp(stamp Stamp) Stamp {
	stamp.SourceRange = cloneRange(stamp.SourceRange)
	stamp.MediaReference = cloneMedia(stamp.MediaReference)
	stamp.Duration = cloneTime(stamp.Duration)
	stamp.Metadata = cloneMetadata(stamp.Metadata)
	return stamp
}
