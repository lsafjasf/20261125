// Package caps models what each target text-track format can express.
package caps

// Level is the fidelity with which a format can express a property.
type Level int

const (
	None   Level = iota // cannot be expressed at all
	Approx              // expressible only with visible degradation
	Exact               // natively expressible
)

func (l Level) String() string {
	switch l {
	case Exact:
		return "exact"
	case Approx:
		return "approx"
	default:
		return "none"
	}
}

// Capability pairs a fidelity level with a short rationale.
type Capability struct {
	Level Level
	Note  string
}

// Profile is the capability table of one target format.
type Profile struct {
	Name string
	Caps map[string]Capability
}

// Property keys used across the audit.
const (
	PropText     = "text"
	PropTiming   = "timing"
	PropFill     = "fillColor"
	PropFontSize = "fontSize"
	PropAlign    = "align"
	PropOutline  = "outline"
	PropShadow   = "shadow"
	PropFade     = "fade"
	PropPosition = "absolutePosition"
	PropRotation = "rotation"
	PropOverlap  = "overlap"
)

// Profiles returns the built-in capability profiles keyed by format name.
func Profiles() map[string]Profile {
	return map[string]Profile{
		"srt": {
			Name: "srt",
			Caps: map[string]Capability{
				PropText:     {Exact, "plain text cues"},
				PropTiming:   {Exact, "start/end per cue"},
				PropFill:     {Approx, "<font color> is a non-standard extension, many players ignore it"},
				PropFontSize: {None, "no font size control"},
				PropAlign:    {None, "no alignment control"},
				PropOutline:  {None, "no stroke/outline concept"},
				PropShadow:   {None, "no shadow concept"},
				PropFade:     {None, "no transition effects"},
				PropPosition: {None, "cues always stack at the default position"},
				PropRotation: {None, "no rotation"},
				PropOverlap:  {Approx, "simultaneous cues stack in player-defined order"},
			},
		},
		"webvtt": {
			Name: "webvtt",
			Caps: map[string]Capability{
				PropText:     {Exact, "plain text plus limited tags"},
				PropTiming:   {Exact, "start/end per cue"},
				PropFill:     {Exact, "<c.color> classes or ::cue CSS"},
				PropFontSize: {Approx, "::cue font-size is player CSS, not carried by the file"},
				PropAlign:    {Exact, "align setting"},
				PropOutline:  {Approx, "no stroke; ::cue text-shadow can fake a thin outline"},
				PropShadow:   {Approx, "::cue text-shadow, player CSS only"},
				PropFade:     {None, "no per-cue fade; CSS transitions on cues are unreliable"},
				PropPosition: {Approx, "line/position are percentages and snap, not pixel-exact"},
				PropRotation: {None, "no rotation"},
				PropOverlap:  {Approx, "overlapping cues are re-stacked to avoid collision"},
			},
		},
		"ass": {
			Name: "ass",
			Caps: map[string]Capability{
				PropText:     {Exact, "dialogue events with override tags"},
				PropTiming:   {Exact, "start/end per event"},
				PropFill:     {Exact, `PrimaryColour / \c`},
				PropFontSize: {Exact, `style fontsize / \fs`},
				PropAlign:    {Exact, "alignment 1-9"},
				PropOutline:  {Exact, `Outline + OutlineColour (\bord)`},
				PropShadow:   {Exact, `Shadow (\shad)`},
				PropFade:     {Exact, `\fad(in,out)`},
				PropPosition: {Exact, `\pos(x,y) pixel-exact`},
				PropRotation: {Approx, `\frz rotates around the text axis only`},
				PropOverlap:  {Exact, "independent positioned events"},
			},
		},
		"ttml": {
			Name: "ttml",
			Caps: map[string]Capability{
				PropText:     {Exact, "p/span elements"},
				PropTiming:   {Exact, "begin/end attributes"},
				PropFill:     {Exact, "tts:color"},
				PropFontSize: {Exact, "tts:fontSize"},
				PropAlign:    {Exact, "tts:textAlign"},
				PropOutline:  {Exact, "tts:textOutline"},
				PropShadow:   {Approx, "tts:textShadow is TTML2, spotty player support"},
				PropFade:     {Approx, "opacity animation is TTML2, spotty player support"},
				PropPosition: {Exact, "region origin/extent in px"},
				PropRotation: {None, "no rotation in TTML1"},
				PropOverlap:  {Exact, "independent regions"},
			},
		},
	}
}

// Get returns the capability for a property, defaulting to None.
func (p Profile) Get(prop string) Capability {
	if c, ok := p.Caps[prop]; ok {
		return c
	}
	return Capability{Level: None, Note: "unknown property"}
}
