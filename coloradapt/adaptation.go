package coloradapt

import "math"

const epsilon = 1e-12

var (
	ACESWhite = Vec3{0.952646074569920, 1.0, 1.008825184351586}
	D50White  = Vec3{0.9642, 1.0, 0.8249}
)

var XYZFromAP0 = Mat3{
	{1.049813942169072, 0.0, -0.000097484503818264},
	{0.0, 1.0, 0.0},
	{-0.000097484503818264, 0.0, 1.044976067389279},
}

var AP0FromXYZ = InverseMat3(XYZFromAP0)

var identityCone = Mat3{
	{1, 0, 0},
	{0, 1, 0},
	{0, 0, 1},
}

var bradfordCone = Mat3{
	{0.8951, 0.2664, -0.1614},
	{-0.7502, 1.7135, 0.0367},
	{0.0389, -0.0685, 1.0296},
}

var vonKriesCone = Mat3{
	{0.40024, 0.70760, -0.08081},
	{-0.22630, 1.16532, 0.04570},
	{0.00000, 0.00000, 0.91822},
}

type Method string

const (
	MethodKeepXYZ  Method = "keep_xyz"
	MethodBradford Method = "bradford"
	MethodVonKries Method = "von_kries"
)

func coneMatrix(method Method) Mat3 {
	switch method {
	case MethodBradford:
		return bradfordCone
	case MethodVonKries:
		return vonKriesCone
	case MethodKeepXYZ:
		return identityCone
	default:
		panic("unknown chromatic adaptation method")
	}
}

func AdaptationMatrix(method Method, source, destination Vec3) Mat3 {
	cone := coneMatrix(method)
	if method == MethodKeepXYZ {
		return identityCone
	}
	sourceCone := cone.MulVec(source)
	destinationCone := cone.MulVec(destination)
	scale := Vec3{
		destinationCone[0] / sourceCone[0],
		destinationCone[1] / sourceCone[1],
		destinationCone[2] / sourceCone[2],
	}
	return MatMul(MatMul(InverseMat3(cone), Diag(scale)), cone)
}

func XYZFromAP0Sample(ap0 Vec3) Vec3 {
	return XYZFromAP0.MulVec(ap0)
}

func AP0SampleFromXYZ(xyz Vec3) Vec3 {
	return AP0FromXYZ.MulVec(xyz)
}

func D50XYZFromAP0(ap0 Vec3, method Method) Vec3 {
	xyz := XYZFromAP0Sample(ap0)
	return AdaptationMatrix(method, ACESWhite, D50White).MulVec(xyz)
}

func AP0FromD50XYZ(xyz Vec3, method Method) Vec3 {
	sourceXYZ := AdaptationMatrix(method, D50White, ACESWhite).MulVec(xyz)
	return AP0SampleFromXYZ(sourceXYZ)
}

func RelativeDifference(want, got Vec3) Vec3 {
	out := Vec3{}
	for channel := 0; channel < 3; channel++ {
		if math.Abs(want[channel]) < epsilon {
			out[channel] = got[channel] - want[channel]
		} else {
			out[channel] = (got[channel] - want[channel]) / want[channel]
		}
	}
	return out
}
