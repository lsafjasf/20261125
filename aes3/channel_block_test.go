package aes3

import "testing"

func TestChannelStatus192BlockAssembly(t *testing.T) {
	report, err := Decode(encodeTestStream(testSpecs(192)))
	if err != nil {
		t.Fatal(err)
	}
	if len(report.ChannelStatus.CompleteLeft192) != 24 {
		t.Fatalf("left block bytes = %d", len(report.ChannelStatus.CompleteLeft192))
	}
	if len(report.ChannelStatus.CompleteRight192) != 24 {
		t.Fatalf("right block bytes = %d", len(report.ChannelStatus.CompleteRight192))
	}
	for i, b := range report.ChannelStatus.CompleteLeft192 {
		if b != 0x55 {
			t.Fatalf("left byte %d = 0x%02x, want 0x55", i, b)
		}
	}
	for i, b := range report.ChannelStatus.CompleteRight192 {
		if b != 0xAA {
			t.Fatalf("right byte %d = 0x%02x, want 0xAA", i, b)
		}
	}
}
