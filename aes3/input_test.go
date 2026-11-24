package aes3

import (
	"strings"
	"testing"
)

func TestParseEdgesJSONVariants(t *testing.T) {
	cases := []string{
		`[0, 1, 2]`,
		`[{"time":0},{"t":1},{"at":2}]`,
		`{"edges":[[0,1],[1,0],[2,1]]}`,
		`{"timestamps":[0,1,2]}`,
	}
	for _, input := range cases {
		edges, err := ParseEdgesJSON(strings.NewReader(input))
		if err != nil {
			t.Fatalf("ParseEdgesJSON(%s): %v", input, err)
		}
		if len(edges) != 3 || edges[2].Time != 2 {
			t.Fatalf("bad edges %#v from %s", edges, input)
		}
	}

	edges, err := ParseEdgesJSON(strings.NewReader(`{"edges":[{"time":1,"level":"rising"}]}`))
	if err != nil {
		t.Fatal(err)
	}
	if edges[0].Level != LevelHigh {
		t.Fatalf("rising level = %v", edges[0].Level)
	}
}

func TestParseEdgesCSVHeaderAndDuration(t *testing.T) {
	input := "timestamp,direction\n0s,rising\n500ms,falling\n1s,rising\n"
	edges, err := ParseEdgesCSV(strings.NewReader(input))
	if err != nil {
		t.Fatal(err)
	}
	if len(edges) != 3 {
		t.Fatalf("edges = %d", len(edges))
	}
	if edges[1].Time != 0.5 || edges[1].Level != LevelLow {
		t.Fatalf("second edge = %+v", edges[1])
	}
}

func TestParseEdgesJSONLines(t *testing.T) {
	input := `{"time":0,"level":1}
{"time":1,"level":0}
{"time":2,"level":1}
`
	edges, err := ParseEdgesJSONLines(strings.NewReader(input))
	if err != nil {
		t.Fatal(err)
	}
	if len(edges) != 3 || edges[0].Level != LevelHigh {
		t.Fatalf("edges=%#v err=%v", edges, err)
	}
}

func TestParseEdgesWhitespaceTable(t *testing.T) {
	edges, err := ParseEdgesCSV(strings.NewReader("0 1\n1 0\n2 1\n"))
	if err != nil {
		t.Fatal(err)
	}
	if len(edges) != 3 || edges[1].Time != 1 || edges[1].Level != LevelLow {
		t.Fatalf("whitespace edges = %#v", edges)
	}
}
