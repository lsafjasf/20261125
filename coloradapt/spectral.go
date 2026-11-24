package coloradapt

import (
	"math"
	"strings"
)

type Illuminant struct {
	Name      string
	Kelvin    float64
	White     Vec3
	intensity func(wavelength float64) float64
}

type Sample struct {
	ID          string
	Group       string
	Illuminant  string
	AP0         Vec3
	SourceWhite Vec3
	SourceLab   Lab
}

func BuildSamples() []Sample {
	illuminants := []Illuminant{
		{Name: "A_2856K", Kelvin: 2856, intensity: planckSpectrum(2856)},
		{Name: "warm_3500K", Kelvin: 3500, intensity: planckSpectrum(3500)},
		{Name: "neutral_5000K", Kelvin: 5000, intensity: planckSpectrum(5000)},
		{Name: "daylight_6500K", Kelvin: 6500, intensity: planckSpectrum(6500)},
		{Name: "synthetic_equal_power", Kelvin: 0, intensity: equalEnergySpectrum},
	}

	reflectances := map[string]func(float64) float64{
		"neutral_18":    constantReflectance(0.18),
		"neutral_50":    constantReflectance(0.50),
		"neutral_90":    constantReflectance(0.90),
		"skin_light":    lightSkinReflectance,
		"skin_medium":   mediumSkinReflectance,
		"skin_deep":     deepSkinReflectance,
		"chroma_red":    bandReflectance(620, 24),
		"chroma_green":  bandReflectance(520, 24),
		"chroma_blue":   bandReflectance(455, 20),
		"chroma_yellow": bandReflectance(575, 22),
		"chroma_cyan":   bandReflectance(490, 20),
	}

	samples := make([]Sample, 0, len(illuminants)*len(reflectances)+26)
	for _, illuminant := range illuminants {
		illuminant.White = spectralXYZ(illuminant, constantReflectance(1))
		for name, reflectance := range reflectances {
			xyz := spectralXYZ(illuminant, reflectance)
			sourceLab := LabFromXYZ(xyz, illuminant.White)
			encoded := AP0SampleFromXYZ(AdaptationMatrix(MethodBradford, illuminant.White, ACESWhite).MulVec(xyz))
			group := groupFromName(name)
			samples = append(samples, Sample{
				ID:          illuminant.Name + "/" + name,
				Group:       group,
				Illuminant:  illuminant.Name,
				AP0:         encoded,
				SourceWhite: illuminant.White,
				SourceLab:   sourceLab,
			})
		}
	}

	samples = append(samples, encodingCubeSamples()...)
	return append(samples, ap0GamutBoundarySamples()...)
}

func groupFromName(name string) string {
	switch {
	case strings.HasPrefix(name, "neutral"):
		return "neutral"
	case strings.HasPrefix(name, "skin"):
		return "skin"
	default:
		return "spectral_chroma"
	}
}

func spectralXYZ(illuminant Illuminant, reflectance func(float64) float64) Vec3 {
	var xyz Vec3
	var whiteY float64
	for wavelength := 380.0; wavelength <= 780.0; wavelength += 5 {
		power := illuminant.intensity(wavelength)
		factor := power * reflectance(wavelength)
		xBar, yBar, zBar := cmf1931(wavelength)
		xyz[0] += factor * xBar
		xyz[1] += factor * yBar
		xyz[2] += factor * zBar
		whiteY += power * yBar
	}
	xyz[0] /= whiteY
	xyz[1] /= whiteY
	xyz[2] /= whiteY
	return xyz
}

// cmf1931 uses a compact analytic approximation so the benchmark has no data
// dependencies. The matrix comparison is exact; these spectral samples are
// deterministic stress cases rather than a replacement for a CIE data table.
func cmf1931(wavelength float64) (float64, float64, float64) {
	xBar := 1.056*gaussian(wavelength, 442.0, 16.0, 26.7) +
		0.362*gaussian(wavelength, 599.8, 37.9, 31.0) -
		0.065*gaussian(wavelength, 501.1, 20.4, 26.2)
	yBar := 0.821*gaussian(wavelength, 568.8, 46.9, 40.5) +
		0.286*gaussian(wavelength, 530.9, 16.3, 31.1)
	zBar := 1.217*gaussian(wavelength, 437.0, 11.8, 36.0) +
		0.681*gaussian(wavelength, 459.0, 26.0, 13.8)
	return maxFloat(xBar, 0), maxFloat(yBar, 0), maxFloat(zBar, 0)
}

func gaussian(x, mean, leftSigma, rightSigma float64) float64 {
	sigma := rightSigma
	if x < mean {
		sigma = leftSigma
	}
	return math.Exp(-0.5 * math.Pow((x-mean)/sigma, 2))
}

func planckSpectrum(kelvin float64) func(float64) float64 {
	return func(wavelength float64) float64 {
		const c2 = 1.438776877e-2
		meters := wavelength * 1e-9
		return 1 / (math.Pow(meters, 5) * (math.Exp(c2/(meters*kelvin)) - 1))
	}
}

func equalEnergySpectrum(wavelength float64) float64 {
	return 1
}

func constantReflectance(value float64) func(float64) float64 {
	return func(float64) float64 { return value }
}

func bandReflectance(center, sigma float64) func(float64) float64 {
	return func(wavelength float64) float64 {
		return 0.02 + 0.96*math.Exp(-0.5*math.Pow((wavelength-center)/sigma, 2))
	}
}

func lightSkinReflectance(wavelength float64) float64 {
	value := 0.30 + 0.35*logistic((wavelength-560)/35) -
		0.08*gaussian(wavelength, 545, 18, 18) +
		0.04*gaussian(wavelength, 580, 15, 15)
	return clamp01(value)
}

func mediumSkinReflectance(wavelength float64) float64 {
	value := 0.14 + 0.32*logistic((wavelength-565)/38) -
		0.05*gaussian(wavelength, 545, 18, 18)
	return clamp01(value)
}

func deepSkinReflectance(wavelength float64) float64 {
	value := 0.05 + 0.22*logistic((wavelength-570)/42)
	return clamp01(value)
}

func logistic(value float64) float64 {
	return 1 / (1 + math.Exp(-value))
}

func clamp01(value float64) float64 {
	return math.Min(1, math.Max(0, value))
}

func maxFloat(a, b float64) float64 {
	return math.Max(a, b)
}
