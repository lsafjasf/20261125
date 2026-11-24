package colorcal

import (
	"errors"
	"math"
)

var errSingularMatrix = errors.New("singular matrix")

func solveDense(input [][]float64, rhs []float64) ([]float64, error) {
	n := len(rhs)
	a := make([][]float64, n)
	for i := range a {
		a[i] = append(append([]float64(nil), input[i]...), rhs[i])
	}

	for col := 0; col < n; col++ {
		pivot := col
		for row := col + 1; row < n; row++ {
			if math.Abs(a[row][col]) > math.Abs(a[pivot][col]) {
				pivot = row
			}
		}
		if math.Abs(a[pivot][col]) < 1e-15 {
			return nil, errSingularMatrix
		}
		a[col], a[pivot] = a[pivot], a[col]

		for row := col + 1; row < n; row++ {
			factor := a[row][col] / a[col][col]
			if factor == 0 {
				continue
			}
			for k := col; k <= n; k++ {
				a[row][k] -= factor * a[col][k]
			}
		}
	}

	x := make([]float64, n)
	for i := n - 1; i >= 0; i-- {
		sum := a[i][n]
		for j := i + 1; j < n; j++ {
			sum -= a[i][j] * x[j]
		}
		x[i] = sum / a[i][i]
		if math.IsNaN(x[i]) || math.IsInf(x[i], 0) {
			return nil, errSingularMatrix
		}
	}
	return x, nil
}

// jacobiSymmetric returns eigenvalues and eigenvectors for a symmetric matrix.
// eigenvectors[i][j] is component i of eigenvector j? We choose columns:
// vectors[row][col], and eigenvalues[col].
func jacobiSymmetric(input [][]float64) ([]float64, [][]float64) {
	n := len(input)
	a := make([][]float64, n)
	v := make([][]float64, n)
	for i := 0; i < n; i++ {
		a[i] = append([]float64(nil), input[i]...)
		v[i] = make([]float64, n)
		v[i][i] = 1
	}

	for sweep := 0; sweep < 100; sweep++ {
		off := 0.0
		for i := 0; i < n; i++ {
			for j := i + 1; j < n; j++ {
				off += a[i][j] * a[i][j]
			}
		}
		if off < 1e-24 {
			break
		}

		for p := 0; p < n-1; p++ {
			for q := p + 1; q < n; q++ {
				apq := a[p][q]
				if math.Abs(apq) < 1e-18 {
					continue
				}
				app, aqq := a[p][p], a[q][q]
				phi := 0.5 * math.Atan2(2*apq, aqq-app)
				c, s := math.Cos(phi), math.Sin(phi)

				for k := 0; k < n; k++ {
					akp, akq := a[k][p], a[k][q]
					a[k][p] = c*akp - s*akq
					a[k][q] = s*akp + c*akq
				}
				for k := 0; k < n; k++ {
					apr, aqr := a[p][k], a[q][k]
					a[p][k] = c*apr - s*aqr
					a[q][k] = s*apr + c*aqr
				}
				for k := 0; k < n; k++ {
					vkp, vkq := v[k][p], v[k][q]
					v[k][p] = c*vkp - s*vkq
					v[k][q] = s*vkp + c*vkq
				}
			}
		}
	}

	eigenvalues := make([]float64, n)
	for i := 0; i < n; i++ {
		eigenvalues[i] = a[i][i]
	}
	return eigenvalues, v
}

func pseudoInverseSymmetric(a [][]float64, relativeCutoff float64) ([][]float64, float64, float64) {
	n := len(a)
	eigenvalues, vectors := jacobiSymmetric(a)
	maxEig := 0.0
	minPositive := math.Inf(1)
	for _, ev := range eigenvalues {
		ev = math.Abs(ev)
		if ev > maxEig {
			maxEig = ev
		}
		if ev > 1e-18 && ev < minPositive {
			minPositive = ev
		}
	}
	if math.IsInf(minPositive, 1) {
		minPositive = 0
	}
	cutoff := maxEig * relativeCutoff

	invEigen := make([]float64, n)
	for i, ev := range eigenvalues {
		if ev > cutoff {
			invEigen[i] = 1 / ev
		}
	}

	out := make([][]float64, n)
	for i := range out {
		out[i] = make([]float64, n)
		for j := 0; j < n; j++ {
			sum := 0.0
			for k := 0; k < n; k++ {
				sum += vectors[i][k] * invEigen[k] * vectors[j][k]
			}
			out[i][j] = sum
		}
	}

	condition := math.Inf(1)
	if minPositive > 0 && maxEig > 0 {
		condition = maxEig / minPositive
	}
	return out, condition, minPositive
}
