package anc

import "testing"

func TestEvenParity(t *testing.T) {
	cases := []struct {
		b    byte
		want byte
	}{
		{0x00, 0}, // zero bits set -> even -> parity bit 0
		{0x01, 1}, // one bit -> parity 1
		{0x02, 1},
		{0x61, 1}, // bits 0,5,6 -> odd
		{0xFF, 0}, // eight bits -> even
		{0x0F, 0}, // four bits -> even
	}
	for _, tc := range cases {
		if got := EvenParity(tc.b); got != tc.want {
			t.Errorf("EvenParity(%02X)=%d want %d", tc.b, got, tc.want)
		}
	}
}

func TestWithParityRoundTrip(t *testing.T) {
	for _, b := range []byte{0x00, 0x01, 0x61, 0xAB, 0xFF, 0x0F, 0xF0} {
		w := WithParity(b)
		if w.Data() != b {
			t.Fatalf("data mismatch: %04X", w)
		}
		if !ParityOK(w) {
			t.Fatalf("ParityOK false for %03X (input %02X)", w, b)
		}
		// Corrupting b8 or b9 must be detected.
		if ParityOK(w ^ (1 << 8)) {
			t.Fatalf("b8 corruption not detected for %03X", w)
		}
		if ParityOK(w ^ (1 << 9)) {
			t.Fatalf("b9 corruption not detected for %03X", w)
		}
	}
}

// TestKnownPacketVector verifies the checksum against a hand-derived ST 291
// vector for DID=61 SDID=01 DC=02 payload AA 55.
//
//	DID 0x61 odd popcount -> 161h
//	SDID 0x01             -> 101h
//	DC  0x02              -> 102h
//	sum9 = 161+101+102+0AA+055 = 463h -> &1FFh = 063h
//	CS bit8=0 -> b9=1      -> 263h
func TestKnownPacketVector(t *testing.T) {
	words := []Word{0x000, 0x3FF, 0x3FF, 0x161, 0x101, 0x102, 0x0AA, 0x055, 0x263}
	if !ParityOK(words[3]) || !ParityOK(words[4]) || !ParityOK(words[5]) {
		t.Fatal("hand-built header parity expected OK")
	}
	if !ChecksumFramingOK(words[8]) {
		t.Fatal("hand-built CS framing expected OK")
	}
	got := Checksum9(words[3:8]...)
	if got != 0x063 {
		t.Fatalf("checksum=%03X want 063", got)
	}
	if MakeChecksumWord(got) != 0x263 {
		t.Fatalf("cs word=%03X want 263", MakeChecksumWord(got))
	}
}

func TestChecksum8(t *testing.T) {
	// 61+01+02+AA+55 = 0x163 -> wrapping byte 0x63.
	if got := Checksum8(0x61, 0x01, 0x02, []byte{0xAA, 0x55}); got != 0x63 {
		t.Fatalf("Checksum8=%02X want 63", got)
	}
}
