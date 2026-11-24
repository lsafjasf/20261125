package review

import (
	"strings"
	"testing"
)

func TestHashIsIndependentOfEntryOrder(t *testing.T) {
	first := &File{Schema: Schema, Entries: []Entry{
		{FrameID: "f02", Decision: DecisionClear, Note: "flare"},
		{FrameID: "f01", Decision: DecisionConfirmTear},
	}}
	second := &File{Entries: []Entry{
		{FrameID: "f01", Decision: "confirm", Note: "  "},
		{FrameID: "f02", Decision: "not_tear", Note: "flare"},
	}}
	firstHash, err := Hash(first)
	if err != nil {
		t.Fatal(err)
	}
	secondHash, err := Hash(second)
	if err != nil {
		t.Fatal(err)
	}
	if firstHash != secondHash {
		t.Fatalf("hashes differ: %s vs %s", firstHash, secondHash)
	}
}

func TestSetRemoveAndCanonicalMarshal(t *testing.T) {
	file := NewFile()
	var err error
	file, err = Set(file, Entry{FrameID: "f10", Decision: "clear", Reviewer: "bob"})
	if err != nil {
		t.Fatal(err)
	}
	file, err = Set(file, Entry{FrameID: "f02", Decision: "uncertain", Note: "look again"})
	if err != nil {
		t.Fatal(err)
	}
	if file.Entries[0].FrameID != "f02" {
		t.Fatalf("entries are not sorted: %+v", file.Entries)
	}
	file, err = Remove(file, "f10")
	if err != nil {
		t.Fatal(err)
	}
	raw, err := Marshal(file)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), `"schema": "tear-review/v1"`) ||
		!strings.Contains(string(raw), `"frame_id": "f02"`) ||
		strings.Contains(string(raw), `"f10"`) {
		t.Fatalf("canonical review JSON = %s", raw)
	}
}

func TestUnknownDecisionFails(t *testing.T) {
	if _, err := Set(NewFile(), Entry{FrameID: "f01", Decision: "maybe-tear"}); err == nil {
		t.Fatal("expected unknown decision to fail")
	}
}
