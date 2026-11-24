package display

import (
	"fmt"
	"math"
)

// Primaries are CIE 1931 xy chromaticities plus the white point.
type Primaries struct {
	RedX, RedY     float64
	GreenX, GreenY float64
	BlueX, BlueY   float64
	WhiteX, WhiteY float64
}

// Display is one row of the display capability table. Peak and Black are
// measured in nits (cd/m^2); Bits is the panel code-value bit depth.
type Display struct {
	ID        string
	PeakNits  float64
	BlackNits float64
	Gamut     Primaries
	Bits      int
}

// ValidationError explains why a capability row is illegal or incomplete.
type ValidationError struct {
	Field  string
	Reason string
}

func (e ValidationError) Error() string {
	return fmt.Sprintf("%s: %s", e.Field, e.Reason)
}

// Validate enforces physical and completeness constraints on the capability
// row. It returns every problem it finds rather than only the first one, so a
// single decision can record all fallback reasons.
func (d Display) Validate() []ValidationError {
	var errs []ValidationError
	add := func(field, format string, args ...any) {
		errs = append(errs, ValidationError{Field: field, Reason: fmt.Sprintf(format, args...)})
	}
	if d.ID == "" {
		add("id", "missing device identifier")
	}
	switch {
	case math.IsNaN(d.PeakNits) || math.IsInf(d.PeakNits, 0):
		add("peak_nits", "value must be a finite number")
	case d.PeakNits <= 0:
		add("peak_nits", "peak luminance must be greater than 0 nits, got %g", d.PeakNits)
	case d.PeakNits > 100000:
		add("peak_nits", "peak luminance %g nits exceeds any physical panel (limit 100000)", d.PeakNits)
	}
	switch {
	case math.IsNaN(d.BlackNits) || math.IsInf(d.BlackNits, 0):
		add("black_nits", "value must be a finite number")
	case d.BlackNits < 0:
		add("black_nits", "black level cannot be negative, got %g", d.BlackNits)
	case d.PeakNits > 0 && d.BlackNits >= d.PeakNits:
		add("black_nits", "black level %g must be below peak %g", d.BlackNits, d.PeakNits)
	}
	switch {
	case d.Bits == 0:
		add("bits", "bit depth is missing")
	case d.Bits < 6 || d.Bits > 16:
		add("bits", "bit depth %d is outside the supported 6..16 range", d.Bits)
	}
	errs = append(errs, d.Gamut.validate()...)
	return errs
}

func (p Primaries) validate() []ValidationError {
	var errs []ValidationError
	xy := map[string][2]float64{
		"red":   {p.RedX, p.RedY},
		"green": {p.GreenX, p.GreenY},
		"blue":  {p.BlueX, p.BlueY},
		"white": {p.WhiteX, p.WhiteY},
	}
	for name, c := range xy {
		x, y := c[0], c[1]
		switch {
		case math.IsNaN(x) || math.IsNaN(y):
			errs = append(errs, ValidationError{Field: "gamut", Reason: name + " chromaticity is missing or NaN"})
		case x <= 0 || x >= 1 || y <= 0 || y >= 1:
			errs = append(errs, ValidationError{Field: "gamut", Reason: fmt.Sprintf("%s chromaticity (%.4f,%.4f) is outside the open unit square", name, x, y)})
		}
	}
	if len(errs) > 0 {
		return errs
	}
	rx, ry := p.RedX, p.RedY
	gx, gy := p.GreenX, p.GreenY
	bx, by := p.BlueX, p.BlueY
	area := rx*(gy-by) + gx*(by-ry) + bx*(ry-gy)
	if math.Abs(area) < 1e-6 {
		errs = append(errs, ValidationError{Field: "gamut", Reason: "primaries are collinear and do not enclose a gamut"})
	}
	return errs
}
