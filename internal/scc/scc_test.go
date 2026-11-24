package scc

import (
	"os"
	"strings"
	"testing"
)

func TestOddParity(t *testing.T) {
	cases := map[byte]bool{0x94: true, 0x26: true, 0x48: false, 0x45: true, 0x00: false, 0xFF: false}
	for b, want := range cases {
		if got := OddParity(b); got != want {
			t.Errorf("OddParity(0x%02X) = %v, want %v", b, got, want)
		}
	}
	if StripParity(0x94) != 0x14 {
		t.Errorf("StripParity(0x94) = 0x%02X, want 0x14", StripParity(0x94))
	}
}

func TestParseCrossFrameCarry(t *testing.T) {
	in := "Scenarist_SCC V1.0\n\n00:00:01:00\tC1\n\n00:00:02:00\tC2 942C\n"
	frames, errs := Parse(strings.NewReader(in))
	if len(errs) != 0 {
		t.Fatalf("unexpected parse errors: %v", errs)
	}
	if len(frames) != 2 {
		t.Fatalf("got %d frames, want 2", len(frames))
	}
	if frames[0].CarryOut == nil || frames[0].CarryOut.Raw != 0xC1 {
		t.Fatalf("frame 0 should carry out 0xC1, got %+v", frames[0].CarryOut)
	}
	if len(frames[1].Bytes) != 4 || !frames[1].Bytes[0].CarriedIn || frames[1].Bytes[0].Raw != 0xC1 {
		t.Fatalf("frame 1 should start with carried 0xC1, got %+v", frames[1].Bytes)
	}
}

func TestParseBadToken(t *testing.T) {
	frames, errs := Parse(strings.NewReader("00:00:01:00\tZZZZ 9426\n"))
	if len(errs) != 1 {
		t.Fatalf("want 1 parse error, got %v", errs)
	}
	if len(frames) != 1 || len(frames[0].Bytes) != 2 {
		t.Fatalf("valid word should survive: %+v", frames)
	}
}

func TestDecodePair(t *testing.T) {
	c := DecodePair(0x14, 0x26)
	if c.Kind != KindControl || c.Name != "RU3" || c.Channel != 1 {
		t.Errorf("RU3 decode: %+v", c)
	}
	c = DecodePair(0x14, 0x60)
	if c.Kind != KindPAC || c.Row != 14 || c.Channel != 1 {
		t.Errorf("PAC row15 decode: %+v", c)
	}
	c = DecodePair(0x19, 0x40)
	if c.Kind != KindPAC || c.Row != 0 || c.Channel != 2 {
		t.Errorf("PAC ch2 row1 decode: %+v", c)
	}
	c = DecodePair(0x14, 0x30)
	if c.Kind != KindUnknown {
		t.Errorf("0x14 0x30 should be unknown, got %+v", c)
	}
}

func feedWords(d *Decoder, tc string, words ...[2]byte) {
	f := Frame{Timecode: tc}
	for _, w := range words {
		f.Bytes = append(f.Bytes,
			Byte{Raw: w[0], Value: StripParity(w[0]), ParityOK: OddParity(w[0])},
			Byte{Raw: w[1], Value: StripParity(w[1]), ParityOK: OddParity(w[1])})
	}
	d.FeedFrame(f)
}

func TestDedupRepeatedControl(t *testing.T) {
	d := NewDecoder(Strict())
	feedWords(d, "00:00:01:00", [2]byte{0x94, 0x26}, [2]byte{0x94, 0x26})
	var dups int
	for _, e := range d.Events {
		if e.Note == "duplicate ignored" {
			dups++
		}
	}
	if dups != 1 {
		t.Errorf("want 1 duplicate-ignored event, got %d", dups)
	}
	if d.Channel(1).Mode != ModeRollUp || d.Channel(1).RollRows != 3 {
		t.Errorf("state after RU3: %+v", d.Channel(1))
	}
}

func TestBadParitySkippedStrictUsedNaive(t *testing.T) {
	strict := NewDecoder(Strict())
	feedWords(strict, "00:00:01:00", [2]byte{0x94, 0x25}) // RU2
	feedWords(strict, "00:00:01:00", [2]byte{0x48, 0x45}) // 0x48 has bad parity
	if got := strict.Channel(1).VisibleText(); len(got) != 0 {
		t.Errorf("strict should skip bad-parity pair, got %v", got)
	}
	naive := NewDecoder(Naive())
	feedWords(naive, "00:00:01:00", [2]byte{0x94, 0x25})
	feedWords(naive, "00:00:01:00", [2]byte{0x48, 0x45})
	if got := naive.Channel(1).VisibleText(); len(got) != 1 || !strings.Contains(got[0], "HE") {
		t.Errorf("naive should render HE, got %v", got)
	}
}

func TestRollUpWindowAndClear(t *testing.T) {
	d := NewDecoder(Strict())
	feedWords(d, "00:00:01:00", [2]byte{0x94, 0x26}) // RU3
	feedWords(d, "00:00:02:00", [2]byte{0xC8, 0x45}) // "HE"
	feedWords(d, "00:00:03:00", [2]byte{0x94, 0xAD}) // CR
	feedWords(d, "00:00:04:00", [2]byte{0x57, 0x4F}) // "WO"
	vis := d.Channel(1).VisibleText()
	if len(vis) != 2 || !strings.Contains(vis[0], "row14 HE") || !strings.Contains(vis[1], "row15 WO") {
		t.Fatalf("roll-up state wrong: %v", vis)
	}
	feedWords(d, "00:00:05:00", [2]byte{0x94, 0x2C}) // EDM
	if got := d.Channel(1).VisibleText(); len(got) != 0 {
		t.Fatalf("EDM should clear display, got %v", got)
	}
}

func TestPopOnFlip(t *testing.T) {
	d := NewDecoder(Strict())
	feedWords(d, "00:00:01:00", [2]byte{0x94, 0x20}) // RCL
	feedWords(d, "00:00:02:00", [2]byte{0xD0, 0x4F}) // "PO" -> non-displayed
	if got := d.Channel(1).VisibleText(); len(got) != 0 {
		t.Fatalf("pop-on text must stay hidden before EOC, got %v", got)
	}
	feedWords(d, "00:00:03:00", [2]byte{0x94, 0x2F}) // EOC
	if got := d.Channel(1).VisibleText(); len(got) != 1 || !strings.Contains(got[0], "PO") {
		t.Fatalf("EOC should flip memory, got %v", got)
	}
}

func TestUnknownCodeKeptAndScanContinues(t *testing.T) {
	d := NewDecoder(Strict())
	feedWords(d, "00:00:01:00",
		[2]byte{0x94, 0xB0}, // unknown control
		[2]byte{0x94, 0x26}) // RU3 still decoded afterwards
	var unknown bool
	for _, e := range d.Events {
		if e.Code.Kind == KindUnknown && strings.Contains(e.Note, "unknown code kept") {
			unknown = true
		}
	}
	if !unknown {
		t.Error("unknown code should be logged and kept")
	}
	if d.Channel(1).Mode != ModeRollUp {
		t.Error("scanning should continue after unknown code")
	}
}

func TestSampleFirstDivergence(t *testing.T) {
	f, err := os.Open("../../testdata/sample.scc")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	frames, errs := Parse(f)
	if len(errs) != 0 {
		t.Fatalf("parse errors: %v", errs)
	}
	_, _, div := Run(frames)
	if div == nil {
		t.Fatal("expected a divergence in the sample file")
	}
	if div.Frame != 2 || div.Channel != 1 {
		t.Errorf("first divergence = frame %d ch%d, want frame 2 ch1", div.Frame, div.Channel)
	}
	if !strings.Contains(div.Naive, "row13|HELLO") {
		t.Errorf("naive should roll twice (HELLO on row 13):\n%s", div.Naive)
	}
}
