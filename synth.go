package colorcal

import (
	"math"
	"math/rand"
	"sort"
)

// Truth is acceptance-only generation truth. It must never be passed to Fit.
type Truth struct {
	Seed              int64          `json:"seed"`
	Exposure          float64        `json:"exposure"`
	WhiteBalance      RGB            `json:"white_balance"`
	Gamma             RGB            `json:"gamma"`
	EffectiveGain     RGB            `json:"effective_gain"`
	ReadNoiseSigma    float64        `json:"read_noise_sigma"`
	ShotNoiseCoeff    float64        `json:"shot_noise_coefficient"`
	FlareByPatch      map[string]RGB `json:"flare_by_patch"`
	ReflectionPatches []string       `json:"reflection_patches"`
	HighlightPatches  []string       `json:"highlight_patches"`
	ShadowPatches     []string       `json:"shadow_patches"`
}

type checkerSpec struct {
	id, name, group, hex string
}

var colorChecker24 = []checkerSpec{
	{"dark-skin", "Dark Skin", "skin", "735244"},
	{"light-skin", "Light Skin", "skin", "C29682"},
	{"blue-sky", "Blue Sky", "chroma", "627A9D"},
	{"foliage", "Foliage", "chroma", "576C43"},
	{"blue-flower", "Blue Flower", "chroma", "8580B1"},
	{"bluish-green", "Bluish Green", "chroma", "67BDAA"},
	{"orange", "Orange", "chroma", "D67E2C"},
	{"purplish-blue", "Purplish Blue", "chroma", "505BA6"},
	{"moderate-red", "Moderate Red", "chroma", "C15A63"},
	{"purple", "Purple", "chroma", "5E3C6C"},
	{"yellow-green", "Yellow Green", "chroma", "9DBC40"},
	{"orange-yellow", "Orange Yellow", "chroma", "E0A426"},
	{"blue", "Blue", "chroma", "383D96"},
	{"green", "Green", "chroma", "469449"},
	{"red", "Red", "chroma", "AF363C"},
	{"yellow", "Yellow", "chroma", "E7C71F"},
	{"magenta", "Magenta", "chroma", "BB5695"},
	{"cyan", "Cyan", "chroma", "0885A1"},
	{"white", "White", "gray-highlight", "F3F3F2"},
	{"neutral-80", "Neutral 80", "gray", "C8C8C8"},
	{"neutral-65", "Neutral 65", "gray", "A0A0A0"},
	{"neutral-50", "Neutral 50", "gray", "7A7A79"},
	{"neutral-35", "Neutral 35", "gray-shadow", "555555"},
	{"black", "Black", "gray-shadow", "343434"},
}

// ColorCheckerReferences returns approximate linearized ColorChecker sRGB refs.
func ColorCheckerReferences() []Patch {
	patches := make([]Patch, 0, len(colorChecker24))
	for _, spec := range colorChecker24 {
		patches = append(patches, Patch{
			ID:           spec.id,
			Name:         spec.name,
			Group:        spec.group,
			ReferenceRGB: hexToLinearRGB(spec.hex),
		})
	}
	return patches
}

// NewSyntheticScenario creates observations and their separately held truth.
// Fit receives only the returned Chart.
func NewSyntheticScenario(seed int64) (Chart, Truth) {
	rawExposure := 1.30
	rawWB := RGB{1.28, 1.0, 1.0 / 1.28}
	exposure, wb := normalizeGauge(rawExposure, rawWB)
	gamma := RGB{0.90, 1.03, 1.12}
	truth := Truth{
		Seed:              seed,
		Exposure:          exposure,
		WhiteBalance:      wb,
		Gamma:             gamma,
		ReadNoiseSigma:    0.0032,
		ShotNoiseCoeff:    0.0075,
		FlareByPatch:      map[string]RGB{},
		ReflectionPatches: []string{"neutral-35", "purple"},
		HighlightPatches:  nil,
		ShadowPatches:     []string{"neutral-35", "black"},
	}
	for c := 0; c < 3; c++ {
		truth.EffectiveGain[c] = exposure * wb[c]
	}
	truth.FlareByPatch["neutral-35"] = RGB{0.11, 0.075, 0.045}
	truth.FlareByPatch["purple"] = RGB{0.075, 0.055, 0.12}

	rng := rand.New(rand.NewSource(seed))
	refs := ColorCheckerReferences()
	chart := Chart{
		SchemaVersion:   "colorcal/v1",
		ValueSpace:      "normalized_linear_reference_and_camera_output",
		SaturationLevel: 1,
		Patches:         make([]Patch, 0, len(refs)),
	}

	for _, source := range refs {
		patch := source
		for c := 0; c < 3; c++ {
			gain := truth.EffectiveGain[c]
			clean := math.Pow(math.Max(gain*patch.ReferenceRGB[c], 0), gamma[c])
			if flare, ok := truth.FlareByPatch[patch.ID]; ok {
				clean += flare[c]
			}
			sigma := math.Hypot(truth.ReadNoiseSigma, truth.ShotNoiseCoeff*math.Sqrt(math.Max(clean, 0)))
			noisy := clean + rng.NormFloat64()*sigma
			if noisy >= 1 {
				noisy = 1
				patch.ClipRGB[c] = true
			}
			if noisy < 0 {
				noisy = 0
			}
			patch.MeasuredRGB[c] = noisy
			patch.SigmaRGB[c] = sigma
		}
		chart.Patches = append(chart.Patches, patch)
	}
	highlightSet := map[string]bool{}
	for _, patch := range chart.Patches {
		for c := 0; c < 3; c++ {
			if patch.ClipRGB[c] || patch.MeasuredRGB[c] >= 0.985 {
				highlightSet[patch.ID] = true
			}
		}
	}
	for id := range highlightSet {
		truth.HighlightPatches = append(truth.HighlightPatches, id)
	}
	sort.Strings(truth.HighlightPatches)
	return chart, truth
}

func hexToLinearRGB(hex string) RGB {
	if len(hex) != 6 {
		return RGB{}
	}
	channel := func(start int) float64 {
		var value int
		for i := 0; i < 2; i++ {
			ch := hex[start+i]
			digit := 0
			switch {
			case ch >= '0' && ch <= '9':
				digit = int(ch - '0')
			case ch >= 'a' && ch <= 'f':
				digit = int(ch-'a') + 10
			case ch >= 'A' && ch <= 'F':
				digit = int(ch-'A') + 10
			}
			value = value*16 + digit
		}
		return SRGBToLinear(float64(value) / 255)
	}
	return RGB{channel(0), channel(2), channel(4)}
}
