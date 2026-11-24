package coloradapt

type Vec3 [3]float64
type Mat3 [3][3]float64

func (m Mat3) MulVec(v Vec3) Vec3 {
	return Vec3{
		m[0][0]*v[0] + m[0][1]*v[1] + m[0][2]*v[2],
		m[1][0]*v[0] + m[1][1]*v[1] + m[1][2]*v[2],
		m[2][0]*v[0] + m[2][1]*v[1] + m[2][2]*v[2],
	}
}

func MatMul(a, b Mat3) Mat3 {
	var out Mat3
	for row := 0; row < 3; row++ {
		for col := 0; col < 3; col++ {
			out[row][col] =
				a[row][0]*b[0][col] +
					a[row][1]*b[1][col] +
					a[row][2]*b[2][col]
		}
	}
	return out
}

func InverseMat3(m Mat3) Mat3 {
	a := m[0][0]
	b := m[0][1]
	c := m[0][2]
	d := m[1][0]
	e := m[1][1]
	f := m[1][2]
	g := m[2][0]
	h := m[2][1]
	i := m[2][2]

	det := a*(e*i-f*h) - b*(d*i-f*g) + c*(d*h-e*g)
	inv := 1 / det
	return Mat3{
		{(e*i - f*h) * inv, (c*h - b*i) * inv, (b*f - c*e) * inv},
		{(f*g - d*i) * inv, (a*i - c*g) * inv, (c*d - a*f) * inv},
		{(d*h - e*g) * inv, (b*g - a*h) * inv, (a*e - b*d) * inv},
	}
}

func Diag(v Vec3) Mat3 {
	return Mat3{
		{v[0], 0, 0},
		{0, v[1], 0},
		{0, 0, v[2]},
	}
}
