package main

import "aes3check/aes3"

// These aliases make the root command package testable by tooling that adds
// package-main tests at the repository root.
type (
	Edge                = aes3.Edge
	Options             = aes3.Options
	Report              = aes3.Report
	Subframe            = aes3.Subframe
	LockInterval        = aes3.LockInterval
	ChannelStatusReport = aes3.ChannelStatusReport
	StatusSegment       = aes3.StatusSegment
)

func Decode(edges []Edge, options ...Options) (*Report, error) {
	return aes3.Decode(edges, options...)
}

func DecodeTimestamps(times []float64, options ...Options) (*Report, error) {
	return aes3.DecodeTimestamps(times, options...)
}

func RecoverClock(edges []Edge, fixedHalfUnit ...float64) (aes3.Clock, error) {
	return aes3.RecoverClock(edges, fixedHalfUnit...)
}
