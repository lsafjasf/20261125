package lut3d

import (
	"errors"
	"math"
	"strings"
	"testing"
)

func TestParseUnitFixture(t *testing.T) {
	lut, err := ParseFile("testdata/valid_unit.cube")
	if err != nil {
		t.Fatalf("ParseFile: %v", err)
	}
	if lut.Title != "unit identity" {
		t.Fatalf("Title = %q", lut.Title)
	}
	if lut.Size != 2 || len(lut.Entries) != 8 {
		t.Fatalf("Size/entries = %d/%d", lut.Size, len(lut.Entries))
	}
	wantDomain := [3]AxisDomain{{0, 1}, {0, 1}, {0, 1}}
	if lut.Domain != wantDomain {
		t.Fatalf("Domain = %#v, want %#v", lut.Domain, wantDomain)
	}
	if lut.Step != [3]float64{1, 1, 1} {
		t.Fatalf("Step = %#v", lut.Step)
	}
}

func TestParseNonUniformFixture(t *testing.T) {
	lut, err := ParseFile("testdata/valid_nonuniform.cube")
	if err != nil {
		t.Fatalf("ParseFile: %v", err)
	}
	wantDomain := [3]AxisDomain{{0.1, 0.9}, {0.2, 1.2}, {0.4, 1.6}}
	if lut.Domain != wantDomain {
		t.Fatalf("Domain = %#v, want %#v", lut.Domain, wantDomain)
	}
	wantStep := [3]float64{0.8, 1.0, 1.2}
	for i := range wantStep {
		if math.Abs(lut.Step[i]-wantStep[i]) > 1e-12 {
			t.Fatalf("Step[%d] = %g, want %g", i, lut.Step[i], wantStep[i])
		}
	}
}

func TestParseAcceptsBOMCommentsAndCRLF(t *testing.T) {
	input := "\ufeffTITLE \"hash # inside\" # outside\r\n" +
		"\r\n" +
		"LUT_3D_SIZE 2 # size\r\n" +
		"0 0 0\r\n1 0 0\r\n0 1 0\r\n1 1 0\r\n" +
		"0 0 1\r\n1 0 1\r\n0 1 1\r\n1 1 1\r\n"
	lut, err := Parse(strings.NewReader(input))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if lut.Title != "hash # inside" {
		t.Fatalf("Title = %q", lut.Title)
	}
}

func TestParseTitleKeepsNonGoEscapes(t *testing.T) {
	lut, err := Parse(strings.NewReader(
		`TITLE "C:\looks\hero"` + "\n" +
			"LUT_3D_SIZE 2\n" +
			"0 0 0\n1 0 0\n0 1 0\n1 1 0\n" +
			"0 0 1\n1 0 1\n0 1 1\n1 1 1\n",
	))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if want := `C:\looks\hero`; lut.Title != want {
		t.Fatalf("Title = %q, want %q", lut.Title, want)
	}
}

func TestParseDefaultDomainIsUnit(t *testing.T) {
	lut, err := Parse(strings.NewReader(
		"LUT_3D_SIZE 2\n" +
			"0 0 0\n1 0 0\n0 1 0\n1 1 0\n" +
			"0 0 1\n1 0 1\n0 1 1\n1 1 1\n",
	))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	want := [3]AxisDomain{{0, 1}, {0, 1}, {0, 1}}
	if lut.Domain != want {
		t.Fatalf("Domain = %#v, want %#v", lut.Domain, want)
	}
}

func TestParseInputRange(t *testing.T) {
	lut, err := Parse(strings.NewReader(
		"LUT_3D_INPUT_RANGE 0.1 0.9\n" +
			"LUT_3D_SIZE 2\n" +
			"0 0 0\n1 0 0\n0 1 0\n1 1 0\n" +
			"0 0 1\n1 0 1\n0 1 1\n1 1 1\n",
	))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	want := [3]AxisDomain{{0.1, 0.9}, {0.1, 0.9}, {0.1, 0.9}}
	if lut.Domain != want {
		t.Fatalf("Domain = %#v, want %#v", lut.Domain, want)
	}
}

func TestParseRejectsInconsistentFixtureWithoutPartialLUT(t *testing.T) {
	lut, err := ParseFile("testdata/inconsistent.cube")
	if err == nil {
		t.Fatal("ParseFile succeeded, want entry-count error")
	}
	if lut != nil {
		t.Fatalf("ParseFile returned partial LUT: %#v", lut)
	}
	pe := requireParseError(t, err, 13, ErrEntryCount)
	if !strings.Contains(pe.Message, "requires 27 entries, got 8") {
		t.Fatalf("message = %q", pe.Message)
	}
}

func TestStrictRejectionsWithLineNumbers(t *testing.T) {
	tests := []struct {
		name string
		in   string
		line int
		kind ErrorKind
		text string
	}{
		{
			name: "short table",
			in:   "UNIT\nLUT_3D_SIZE 2\n0 0 0\n",
			line: 4, kind: ErrEntryCount, text: "requires 8 entries, got 1",
		},
		{
			name: "extra table row",
			in: "LUT_3D_SIZE 2\n" +
				"0 0 0\n1 0 0\n0 1 0\n1 1 0\n0 0 1\n1 0 1\n0 1 1\n1 1 1\n" +
				"2 2 2\n",
			line: 10, kind: ErrEntryCount, text: "already complete",
		},
		{
			name: "bad data number",
			in:   "LUT_3D_SIZE 2\n0 0 0\n1 0 0\n0 1 0\n1 1 0\n0 0 1\n1 0 1\n0 1 1\n1 1 nope\n",
			line: 9, kind: ErrSyntax, text: "invalid number",
		},
		{
			name: "data before size",
			in:   "UNIT\n0 0 0\n",
			line: 2, kind: ErrHeader, text: "before LUT_3D_SIZE",
		},
		{
			name: "duplicate size",
			in:   "LUT_3D_SIZE 2\nLUT_3D_SIZE 2\n",
			line: 2, kind: ErrHeader, text: "duplicate LUT_3D_SIZE",
		},
		{
			name: "unknown directive",
			in:   "UNIT\nLUT_3D_SISE 2\n",
			line: 2, kind: ErrSyntax, text: "unknown directive",
		},
		{
			name: "unit and domain",
			in:   "UNIT\nDOMAIN_MIN 0 0 0\n",
			line: 2, kind: ErrHeader, text: "cannot be combined",
		},
		{
			name: "missing domain max",
			in:   "LUT_3D_SIZE 2\nDOMAIN_MIN 0 0 0\n",
			line: 2, kind: ErrDomain, text: "requires a matching DOMAIN_MAX",
		},
		{
			name: "inverted nonuniform domain",
			in:   "LUT_3D_SIZE 2\nDOMAIN_MIN 0 0 0\nDOMAIN_MAX 1 1 -1\n",
			line: 3, kind: ErrDomain, text: "must be less than maximum",
		},
		{
			name: "inverted domain reversed declaration order",
			in:   "LUT_3D_SIZE 2\nDOMAIN_MAX 1 1 -1\nDOMAIN_MIN 0 0 0\n",
			line: 3, kind: ErrDomain, text: "axis B",
		},
		{
			name: "bad header value",
			in:   "LUT_3D_SIZE 2 2\n",
			line: 1, kind: ErrSyntax, text: "exactly one integer",
		},
		{
			name: "directive after data",
			in:   "LUT_3D_SIZE 2\n0 0 0\nUNIT\n",
			line: 3, kind: ErrSyntax, text: "cannot appear after table data",
		},
		{
			name: "duplicate title",
			in:   "TITLE \"one\"\nTITLE \"two\"\n",
			line: 2, kind: ErrHeader, text: "duplicate TITLE",
		},
		{
			name: "unit and input range",
			in:   "UNIT\nLUT_3D_INPUT_RANGE 0 1\n",
			line: 2, kind: ErrHeader, text: "cannot be combined",
		},
		{
			name: "one point LUT",
			in:   "LUT_3D_SIZE 1\n0 0 0\n",
			line: 1, kind: ErrHeader, text: "at least 2",
		},
		{
			name: "1D LUT",
			in:   "LUT_1D_SIZE 2\n",
			line: 1, kind: ErrUnsupported, text: "1D LUTs are not supported",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lut, err := Parse(strings.NewReader(tt.in))
			if err == nil {
				t.Fatal("Parse succeeded, want error")
			}
			if lut != nil {
				t.Fatalf("Parse returned partial LUT: %#v", lut)
			}
			pe := requireParseError(t, err, tt.line, tt.kind)
			if !strings.Contains(pe.Message, tt.text) {
				t.Fatalf("message = %q, want substring %q", pe.Message, tt.text)
			}
		})
	}
}

func TestParseRejectsInvalidUTF8(t *testing.T) {
	_, err := ParseBytes([]byte{'U', 'N', 'I', 'T', '\n', 0xff, '\n'})
	requireParseError(t, err, 2, ErrSyntax)
}

func requireParseError(t *testing.T, err error, line int, kind ErrorKind) *ParseError {
	t.Helper()
	var pe *ParseError
	if !errors.As(err, &pe) {
		t.Fatalf("error %v is not a *ParseError", err)
	}
	if pe.Line != line {
		t.Fatalf("error line = %d, want %d (%v)", pe.Line, line, err)
	}
	if pe.Kind != kind {
		t.Fatalf("error kind = %q, want %q (%v)", pe.Kind, kind, err)
	}
	return pe
}
