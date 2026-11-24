package colorcal

import "math"

func weightedCost(theta []float64, obs []observation) float64 {
	cost := 0.0
	for i := range obs {
		ob := &obs[i]
		pred := predictTheta(ob.ref, theta, ob.channel)
		r := ob.measured - pred
		cost += ob.weight * r * r / ob.variance
	}
	return cost
}

// scaledJacobian returns rows of sqrt(w)/sigma * d residual/d theta and the
// similarly scaled residual vector.
func scaledJacobian(theta []float64, obs []observation) ([][]float64, []float64) {
	var rows [][]float64
	var residuals []float64

	for i := range obs {
		ob := &obs[i]
		if ob.weight <= 1e-12 {
			continue
		}
		jac := make([]float64, thetaParameters)
		for p := 0; p < thetaParameters; p++ {
			h := 1e-5 * math.Max(1, math.Abs(theta[p]))
			plus := append([]float64(nil), theta...)
			minus := append([]float64(nil), theta...)
			plus[p] += h
			minus[p] -= h
			dp := predictTheta(ob.ref, plus, ob.channel)
			dm := predictTheta(ob.ref, minus, ob.channel)
			jac[p] = (dp - dm) / (2 * h)
		}

		scale := math.Sqrt(ob.weight / ob.variance)
		pred := predictTheta(ob.ref, theta, ob.channel)
		for p := range jac {
			jac[p] *= scale // positive derivative of prediction; RHS is y-prediction
		}
		rows = append(rows, jac)
		residuals = append(residuals, scale*(ob.measured-pred))
	}
	return rows, residuals
}

func normalEquations(rows [][]float64, residuals []float64) ([][]float64, []float64) {
	n := thetaParameters
	a := make([][]float64, n)
	b := make([]float64, n)
	for i := range a {
		a[i] = make([]float64, n)
	}
	for r, row := range rows {
		for i := 0; i < n; i++ {
			b[i] += row[i] * residuals[r]
			for j := 0; j < n; j++ {
				a[i][j] += row[i] * row[j]
			}
		}
	}
	return a, b
}

func lmMinimize(initial []float64, obs []observation, maxIterations int) ([]float64, bool, int) {
	theta := append([]float64(nil), initial...)
	cost := weightedCost(theta, obs)
	lambda := 1e-3
	converged := false

	for iteration := 1; iteration <= maxIterations; iteration++ {
		rows, scaledResiduals := scaledJacobian(theta, obs)
		a, b := normalEquations(rows, scaledResiduals)

		damped := make([][]float64, thetaParameters)
		for i := range a {
			damped[i] = append([]float64(nil), a[i]...)
			damped[i][i] += lambda * math.Max(a[i][i], 1e-8)
		}

		delta, err := solveDense(damped, b)
		if err != nil {
			lambda *= 10
			if lambda > 1e14 {
				return theta, false, iteration
			}
			continue
		}

		candidate := append([]float64(nil), theta...)
		maxDelta := 0.0
		for i := range candidate {
			candidate[i] += delta[i]
			maxDelta = math.Max(maxDelta, math.Abs(delta[i]))
		}
		newCost := weightedCost(candidate, obs)
		if newCost < cost*(1+1e-10) || math.Abs(newCost-cost) < 1e-12 {
			theta = candidate
			if newCost < cost {
				cost = newCost
			}
			lambda = math.Max(lambda/3, 1e-10)
			if maxDelta < 1e-9 {
				converged = true
				return theta, converged, iteration
			}
		} else {
			lambda *= 4
			if lambda > 1e14 {
				return theta, false, iteration
			}
		}
	}
	return theta, converged, maxIterations
}
