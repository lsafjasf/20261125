package cli

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"

	"ancscan/anc"
)

func runGen(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("gen-scenario", flag.ContinueOnError)
	fs.SetOutput(stderr)
	outPath := fs.String("out", "", "write scenario JSON to file instead of stdout")
	if err := fs.Parse(args); err != nil {
		return 2
	}

	doc, err := BuildScenario()
	if err != nil {
		fmt.Fprintf(stderr, "gen error: %v\n", err)
		return 1
	}
	data, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		fmt.Fprintf(stderr, "encode error: %v\n", err)
		return 1
	}
	data = append(data, '\n')

	if *outPath == "" {
		_, err = stdout.Write(data)
		return codeFromErr(err, stderr)
	}
	if err := os.WriteFile(*outPath, data, 0o644); err != nil {
		fmt.Fprintf(stderr, "write error: %v\n", err)
		return 1
	}
	fmt.Fprintf(stdout, "wrote %s\n", *outPath)
	return 0
}

func codeFromErr(err error, stderr io.Writer) int {
	if err != nil {
		fmt.Fprintf(stderr, "write error: %v\n", err)
		return 1
	}
	return 0
}

// BuildScenario constructs the canonical demonstration document. It is
// exported so tests can exercise the exact shipped fixture.
func BuildScenario() (*anc.Document, error) {
	// A valid caption packet, prefixed by 4 non-ADF bytes so byte_offset is 4.
	captionPayload := []byte{0x96, 0x69, 0x4B, 0x0F, 0x01, 0xCE, 0x43, 0x00}
	captionWords, err := anc.BuildWords(0x61, 0x01, captionPayload)
	if err != nil {
		return nil, err
	}
	captionBytes, err := anc.SerializeWords(captionWords, anc.EncU16BE)
	if err != nil {
		return nil, err
	}

	// Unknown Type-1 DID, 8-bit form.
	unknown8, err := anc.BuildANC8(0xCA, 0x10, []byte{0xDE, 0xAD, 0xBE, 0xEF})
	if err != nil {
		return nil, err
	}

	// Time code packet with a deliberately flipped DID parity bit.
	tcWords, err := anc.BuildWords(0x60, 0x01, []byte{0x00, 0x12, 0x34, 0x56})
	if err != nil {
		return nil, err
	}
	tcWords[3] ^= 1 << 8
	tcBad, err := anc.SerializeWords(tcWords, anc.EncU16BE)
	if err != nil {
		return nil, err
	}

	// ANC8 packet with a corrupted checksum byte.
	badCS, err := anc.BuildANC8(0x41, 0x05, []byte{0x08, 0x44})
	if err != nil {
		return nil, err
	}
	badCS[len(badCS)-1] ^= 0xA5

	// Packet split across lines 26/27.
	splitPayload := []byte{0x11, 0x22, 0x33, 0x44, 0x55, 0x66}
	splitWords, err := anc.BuildWords(0x61, 0x01, splitPayload)
	if err != nil {
		return nil, err
	}
	splitBytes, err := anc.SerializeWords(splitWords, anc.EncU16BE)
	if err != nil {
		return nil, err
	}
	splitUnit := 6 + 2 // keep header + first two UDW on line 26
	line26 := splitBytes[:2*splitUnit]
	line27 := splitBytes[2*splitUnit:]

	// Field 2: truncated line followed by a fresh ADF line.
	truncWords, err := anc.BuildWords(0x61, 0x01, []byte{0x01, 0x02, 0x03})
	if err != nil {
		return nil, err
	}
	truncBytes, err := anc.SerializeWords(truncWords, anc.EncU16BE)
	if err != nil {
		return nil, err
	}
	line335 := truncBytes[:2*7] // ADF+header+UDW0, missing 2 UDW + CS

	freshWords, err := anc.BuildWords(0x41, 0x05, []byte{0x08})
	if err != nil {
		return nil, err
	}
	freshBytes, err := anc.SerializeWords(freshWords, anc.EncU16BE)
	if err != nil {
		return nil, err
	}

	// Final line: truncated with no following line.
	lastWords, err := anc.BuildWords(0x77, 0x02, []byte{0xAA, 0xBB, 0xCC, 0xDD})
	if err != nil {
		return nil, err
	}
	lastBytes, err := anc.SerializeWords(lastWords, anc.EncU16BE)
	if err != nil {
		return nil, err
	}
	line338 := lastBytes[:len(lastBytes)-4] // drop last UDW? and CS -> truncated

	doc := &anc.Document{
		Schema:            "ancscan/input-v1",
		Standard:          "625i50",
		Encoding:          anc.EncU16BE,
		ContinuationLines: 1,
		ExpectedLines: map[string][]int{
			"0x60": {13},
			"0x41": {25, 336},
		},
		Lines: []anc.LineDTO{
			{Field: 1, Line: 22, DataHex: "80808080" + hex(captionBytes)},
			{Field: 1, Line: 23, Encoding: "anc8", DataHex: hex(unknown8)},
			{Field: 1, Line: 24, DataHex: hex(tcBad)},
			{Field: 1, Line: 25, Encoding: "anc8", DataHex: hex(badCS)},
			{Field: 1, Line: 26, DataHex: hex(line26)},
			{Field: 1, Line: 27, DataHex: hex(line27)},
			{Field: 2, Line: 335, DataHex: hex(line335)},
			{Field: 2, Line: 336, DataHex: hex(freshBytes)},
			{Field: 2, Line: 337, DataHex: hex(captionBytes)}, // duplicate of line 22 core
			{Field: 2, Line: 338, DataHex: hex(line338)},
		},
	}
	return doc, nil
}

func hex(b []byte) string {
	const d = "0123456789abcdef"
	out := make([]byte, 2*len(b))
	for i, v := range b {
		out[2*i] = d[v>>4]
		out[2*i+1] = d[v&0xF]
	}
	return string(out)
}
