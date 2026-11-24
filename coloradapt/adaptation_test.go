package coloradapt

import (
	"math"
	"testing"
)

func TestChromaticAdaptationMapsWhitePoints(t *testing.T) {
	methods := []Method{MethodBradford, MethodVonKries}
	for _, method := range methods {
		method := method
		t.Run(string(method), func(t *testing.T) {
			adapted := AdaptationMatrix(method, ACESWhite, D50White).MulVec(ACESWhite)
			assertVecNear(t, adapted, D50White, 1e-12)
		})
	}

	kept := AdaptationMatrix(MethodKeepXYZ, ACESWhite, D50White).MulVec(ACESWhite)
	assertVecNear(t, kept, ACESWhite, 1e-12)
	assertMatNear(t, AdaptationMatrix(MethodKeepXYZ, ACESWhite, D50White), Mat3{
		{1, 0, 0},
		{0, 1, 0},
		{0, 0, 1},
	}, 0)
}

func TestBradfordAndVonKriesKnownD65ToD50Matrix(t *testing.T) {
	d65 := Vec3{0.95047, 1, 1.08883}
	bradford := AdaptationMatrix(MethodBradford, d65, D50White)
	wantBradford := Mat3{
		{1.047886, 0.022887, -0.050147},
		{0.029583, 0.990484, -0.017079},
		{-0.009244, 0.015064, 0.751645},
	}
	assertMatNear(t, bradford, wantBradford, 2e-4)

	vonKries := AdaptationMatrix(MethodVonKries, d65, D50White)
	assertVecNear(t, vonKries.MulVec(d65), D50White, 1e-12)
	assertMatNear(t, coneMatrix(MethodVonKries), Mat3{
		{0.40024, 0.70760, -0.08081},
		{-0.22630, 1.16532, 0.04570},
		{0, 0, 0.91822},
	}, 1e-12)
}

func TestRoundTripForAllSamplesAndMethods(t *testing.T) {
	samples := BuildSamples()
	for _, method := range []Method{MethodKeepXYZ, MethodBradford, MethodVonKries} {
		t.Run(string(method), func(t *testing.T) {
			results := Evaluate(method, samples)
			for _, result := range results {
				if result.RoundTripDeltaE > 1e-10 {
					t.Fatalf("%s round-trip ΔE2000 = %.3e", result.Sample.ID, result.RoundTripDeltaE)
				}
				if result.MaxXYZResidual > 1e-12 {
					t.Fatalf("%s XYZ residual = %.3e", result.Sample.ID, result.MaxXYZResidual)
				}
				if result.MaxAP0Residual > 1e-10 {
					t.Fatalf("%s AP0 residual = %.3e", result.Sample.ID, result.MaxAP0Residual)
				}
			}
		})
	}
}

func assertVecNear(t *testing.T, got, want Vec3, tolerance float64) {
	t.Helper()
	for index := range got {
		if math.Abs(got[index]-want[index]) > tolerance {
			t.Fatalf("channel %d = %.12f, want %.12f", index, got[index], want[index])
		}
	}
}

func assertMatNear(t *testing.T, got, want Mat3, tolerance float64) {
	t.Helper()
	for row := 0; row < 3; row++ {
		for col := 0; col < 3; col++ {
			if math.Abs(got[row][col]-want[row][col]) > tolerance {
				t.Fatalf("matrix[%d][%d] = %.12f, want %.12f", row, col, got[row][col], want[row][col])
			}
		}
	}
}
