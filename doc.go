// Package mediaidx maintains an append-only integrity index for long media
// without a database.
//
// Media is appended through a transaction. Each transaction stages one leaf
// per frame or audio sample segment, hashes only those new leaves, and commits
// a Merkle Mountain Range frontier plus a durable data boundary. Opening an
// index restores that frontier directly; old leaves are not rehashed.
//
// A commit record is the visibility boundary. Torn records and leaves written
// before a missing or invalid commit are ignored and truncated during
// recovery, so interrupted work never advances the completed frontier.
//
// Verify streams committed ranges and reports the frame or sample segment
// affected by tail truncation, replacement, gaps, or an uncommitted suffix.
// LocateOffset and LocateRange map arbitrary byte damage back to leaves and
// their media coordinates.
package mediaidx
