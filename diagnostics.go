package colorcal

import (
	"fmt"
	"math"
	"sort"
)

var diagnosticLabels = []string{
	"log_exposure",
	"log_wb_r", "log_wb_g", "log_wb_b",
	"log_gain_r", "log_gain_g", "log_gain_b",
	"log_gamma_r", "log_gamma_g", "log_gamma_b",
}

func diagnosticTransform() [][]float64 {
	m := make([][]float64, len(diagnosticLabels))
	for i := range m {
		m[i] = make([]float64, thetaParameters)
	}

	m[0][0], m[0][1], m[0][2] = 1.0/3, 1.0/3, 1.0/3

	m[1][0], m[1][1], m[1][2] = 2.0/3, -1.0/3, -1.0/3
	m[2][0], m[2][1], m[2][2] = -1.0/3, 2.0/3, -1.0/3
	m[3][0], m[3][1], m[3][2] = -1.0/3, -1.0/3, 2.0/3

	m[4][0] = 1
	m[5][1] = 1
	m[6][2] = 1

	m[7][3] = 1
	m[8][4] = 1
	m[9][5] = 1
	return m
}

func buildDiagnostics(theta []float64, obs []observation, iterations int, converged bool) FitDiagnostics {
	rows, _ := scaledJacobian(theta, obs)
	a, _ := normalEquations(rows, make([]float64, len(rows)))

	sumWeights, chiSquare := 0.0, 0.0
	active := 0
	for i := range obs {
		ob := &obs[i]
		sumWeights += ob.weight
		chiSquare += ob.weight * ob.zScore * ob.zScore
		if ob.weight > 1e-6 {
			active++
		}
	}
	dof := math.Max(1, sumWeights-float64(thetaParameters))
	dispersion := chiSquare / dof
	if math.IsNaN(dispersion) || math.IsInf(dispersion, 0) || dispersion <= 0 {
		dispersion = 1
	}

	invA, condition, minEig := pseudoInverseSymmetric(a, 1e-13)
	// Do not shrink uncertainty below the supplied/robust measurement variance
	// merely because a finite fixture is noiseless or slightly over-fit.
	covarianceScale := math.Max(dispersion, 1.0)
	cov6 := multiplyScalarMatrix(invA, covarianceScale)
	cov := transformCovariance(cov6, diagnosticTransform())
	corr := correlationMatrix(cov)

	diag := FitDiagnostics{
		Observations:         len(obs),
		WeightedObservations: active,
		Parameters:           thetaParameters,
		Iterations:           iterations,
		Converged:            converged,
		ReducedChiSquare:     jsonSafeFloat(dispersion),
		MatrixCondition:      jsonSafeFloat(condition),
		SmallestEigenvalue:   minEig,
		Identifiable:         condition < 1e8 && active >= thetaParameters+3,
		CovarianceLabels:     append([]string(nil), diagnosticLabels...),
		Covariance:           cov,
		Correlation:          corr,
		Notes: []string{
			"WB uses geomean(WB)=1; exposure and WB are only separated under this gauge.",
			"effective_gain_c = exposure * white_balance_c is the directly identifiable quantity.",
			"Clipped channels and robust-zero-weight reflections are excluded from fitting.",
		},
	}
	diag.Couplings = buildCouplings(corr, cov6)
	if condition >= 1e6 {
		diag.Notes = append(diag.Notes, fmt.Sprintf(
			"Jacobian condition %.3g is high: gain/response/exposure combinations are weakly identifiable.", condition))
	}
	if !converged {
		diag.Notes = append(diag.Notes, "optimizer did not report tight convergence; inspect patch residuals and coupling.")
	}
	return diag
}

func jsonSafeFloat(v float64) float64 {
	switch {
	case math.IsNaN(v):
		return 0
	case math.IsInf(v, 1):
		return 1e308
	case math.IsInf(v, -1):
		return -1e308
	default:
		return v
	}
}

func multiplyScalarMatrix(a [][]float64, scalar float64) [][]float64 {
	out := make([][]float64, len(a))
	for i := range a {
		out[i] = make([]float64, len(a[i]))
		for j := range a[i] {
			out[i][j] = a[i][j] * scalar
		}
	}
	return out
}

func transformCovariance(cov [][]float64, transform [][]float64) [][]float64 {
	n := len(transform)
	k := len(cov)
	tmp := make([][]float64, n)
	for i := range tmp {
		tmp[i] = make([]float64, k)
		for j := 0; j < k; j++ {
			sum := 0.0
			for p := 0; p < k; p++ {
				sum += transform[i][p] * cov[p][j]
			}
			tmp[i][j] = sum
		}
	}

	out := make([][]float64, n)
	for i := range out {
		out[i] = make([]float64, n)
		for j := 0; j < n; j++ {
			sum := 0.0
			for p := 0; p < k; p++ {
				sum += tmp[i][p] * transform[j][p]
			}
			out[i][j] = sum
		}
	}
	return out
}

func correlationMatrix(cov [][]float64) [][]float64 {
	n := len(cov)
	corr := make([][]float64, n)
	for i := range corr {
		corr[i] = make([]float64, n)
		ci := math.Max(cov[i][i], 0)
		for j := 0; j < n; j++ {
			cj := math.Max(cov[j][j], 0)
			if ci <= 1e-20 || cj <= 1e-20 {
				corr[i][j] = 0
				continue
			}
			r := cov[i][j] / math.Sqrt(ci*cj)
			corr[i][j] = math.Max(-1, math.Min(1, r))
		}
	}
	return corr
}

func couplingStrength(absR float64) string {
	switch {
	case absR >= 0.95:
		return "critical"
	case absR >= 0.85:
		return "strong"
	case absR >= 0.70:
		return "moderate"
	default:
		return "measurable"
	}
}

func buildCouplings(corr [][]float64, cov6 [][]float64) []Coupling {
	var couplings []Coupling
	seen := map[[2]string]bool{}
	add := func(a, b string, r float64, explain bool) {
		key := [2]string{a, b}
		reverse := [2]string{b, a}
		if seen[key] || seen[reverse] {
			return
		}
		seen[key] = true
		absR := math.Abs(r)
		c := Coupling{Parameters: key, Correlation: r, Strength: couplingStrength(absR)}
		if explain && absR >= 0.85 {
			c.Message = "broad tonal range or an independently anchored exposure target is needed to separate these"
		}
		couplings = append(couplings, c)
	}

	for i := 0; i < len(diagnosticLabels); i++ {
		for j := i + 1; j < len(diagnosticLabels); j++ {
			r := corr[i][j]
			if math.Abs(r) >= 0.65 {
				add(diagnosticLabels[i], diagnosticLabels[j], r, true)
			}
		}
	}

	// Always expose the fundamental gain/response slope ambiguity per channel.
	channels := []string{"r", "g", "b"}
	for c, name := range channels {
		varianceProduct := cov6[c][c] * cov6[c+3][c+3]
		r := 0.0
		if varianceProduct > 1e-24 {
			r = cov6[c][c+3] / math.Sqrt(varianceProduct)
			r = math.Max(-1, math.Min(1, r))
		}
		add("log_gain_"+name, "log_gamma_"+name, r, true)
	}

	sort.SliceStable(couplings, func(i, j int) bool {
		return math.Abs(couplings[i].Correlation) > math.Abs(couplings[j].Correlation)
	})
	if len(couplings) > 14 {
		couplings = couplings[:14]
	}
	return couplings
}
