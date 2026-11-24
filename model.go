package colorcal

import "math"

// channelNames is the stable order used by JSON diagnostics.
var channelNames = [...]string{"r", "g", "b"}

func channelName(c int) string { return channelNames[c] }

// Theta uses log gains and log gammas so every unconstrained optimizer step
// remains positive. The identifiable physical model is
//
//	measured_c = (gain_c * reference_c)^gamma_c + noise
//	gain_c     = exposure * white_balance_c
//
// Exposure and white balance are separated with geomean(WB)=1.
const thetaParameters = 6

func predictTheta(ref float64, theta []float64, channel int) float64 {
	logGain := theta[channel]
	gamma := math.Exp(theta[channel+3])
	linear := math.Exp(logGain) * ref
	if linear <= 0 {
		return 0
	}
	return math.Pow(linear, gamma)
}

func thetaToParams(theta []float64) Params {
	logGain := [3]float64{theta[0], theta[1], theta[2]}
	logExposure := (logGain[0] + logGain[1] + logGain[2]) / 3

	p := Params{
		Model:       "power-response/v1",
		Exposure:    math.Exp(logExposure),
		LogExposure: logExposure,
	}
	for c := 0; c < 3; c++ {
		p.LogGain[c] = logGain[c]
		p.EffectiveGain[c] = math.Exp(logGain[c])
		p.LogWhiteBalance[c] = logGain[c] - logExposure
		p.WhiteBalance[c] = math.Exp(p.LogWhiteBalance[c])
		p.Gamma[c] = math.Exp(theta[c+3])
	}
	return p
}

func correctChannel(measured float64, p Params, channel int) float64 {
	if measured <= 0 {
		return 0
	}
	return math.Pow(measured, 1/p.Gamma[channel]) / p.EffectiveGain[channel]
}

func correctRGB(m RGB, p Params) RGB {
	var out RGB
	for c := 0; c < 3; c++ {
		out[c] = correctChannel(m[c], p, c)
	}
	return out
}

// normalizeGauge returns truth/comparable parameters in the same gauge as the
// estimate. The scalar removed from WB is moved into exposure.
func normalizeGauge(exposure float64, wb RGB) (float64, RGB) {
	logMean := (math.Log(wb[0]) + math.Log(wb[1]) + math.Log(wb[2])) / 3
	normalized := wb
	for c := range normalized {
		normalized[c] = math.Log(wb[c]) - logMean
		normalized[c] = math.Exp(normalized[c])
	}
	return exposure * math.Exp(logMean), normalized
}

func clampUnit(x float64) float64 {
	if x < 0 {
		return 0
	}
	if x > 1 {
		return 1
	}
	return x
}
