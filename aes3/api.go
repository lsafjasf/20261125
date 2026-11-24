package aes3

// DecodeEdges is a descriptive alias for Decode.
func DecodeEdges(edges []Edge, options ...Options) (*Report, error) {
	return Decode(edges, options...)
}

// DecodeTimestamps decodes polarity-unknown edge timestamps.
func DecodeTimestamps(times []float64, options ...Options) (*Report, error) {
	edges := make([]Edge, len(times))
	for i, t := range times {
		edges[i] = Edge{Time: t, Level: LevelUnknown}
	}
	return Decode(edges, options...)
}

// DecodeIntervals interprets intervals as successive edge spacings.
func DecodeIntervals(intervals []float64, options ...Options) (*Report, error) {
	return Decode(EdgesFromIntervals(intervals), options...)
}

// Clock is a recovered bit-clock description.
type Clock struct {
	HalfBitTime float64
	BitCellTime float64
	BitRate     float64
	Source      string
}

// RecoverClock recovers the half-BMC-bit and AES3 bit-cell times.
func RecoverClock(edges []Edge, fixedHalfUnit ...float64) (Clock, error) {
	fixed := 0.0
	if len(fixedHalfUnit) > 0 {
		fixed = fixedHalfUnit[0]
	}
	opt := Options{HalfUnit: fixed}.normalized()
	h, source, err := recoverHalfUnit(prepareEdges(edges), fixed, opt.JitterTolerance)
	if err != nil {
		return Clock{}, err
	}
	return Clock{HalfBitTime: h, BitCellTime: 2 * h, BitRate: bitRate(h), Source: source}, nil
}
