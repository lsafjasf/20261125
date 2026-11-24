package anc_test

import (
	"fmt"

	"ancscan/anc"
)

func ExampleScan() {
	words, _ := anc.BuildWords(0x61, 0x01, []byte{0x01, 0x02})
	raw, _ := anc.SerializeWords(words, anc.EncANC8)

	report, _ := anc.Scan([]anc.Line{{
		Field:    1,
		Number:   21,
		Bytes:    raw,
		Encoding: anc.EncANC8,
	}}, nil)

	p := report.Packets[0]
	fmt.Println(p.Classification, p.Field, p.Line, p.ByteOffset, p.DIDHex, p.PayloadHex)
	// Output: valid_vanc 1 21 0 0x61 0102
}
