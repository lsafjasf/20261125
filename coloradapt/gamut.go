package coloradapt

func ap0GamutBoundarySamples() []Sample {
	primaries := []struct {
		name string
		ap0  Vec3
	}{
		{name: "red", ap0: Vec3{1, 0, 0}},
		{name: "green", ap0: Vec3{0, 1, 0}},
		{name: "blue", ap0: Vec3{0, 0, 1}},
	}
	fractions := []float64{0, 0.125, 0.25, 0.5, 0.75, 0.875, 1}
	offsets := []struct {
		name  string
		value float64
	}{
		{name: "edge", value: 0},
		{name: "near", value: 0.025},
		{name: "inner", value: 0.075},
	}

	var samples []Sample
	for edge := 0; edge < len(primaries); edge++ {
		first := primaries[edge]
		second := primaries[(edge+1)%len(primaries)]
		for _, fraction := range fractions {
			point := interpolate(first.ap0, second.ap0, fraction)
			for _, offset := range offsets {
				coords := addInteriorOffset(point, offset.value)
				group := "ap0_gamut_edge"
				if offset.value > 0 {
					group = "ap0_gamut_near_edge"
				}
				xyz := XYZFromAP0Sample(coords)
				samples = append(samples, Sample{
					ID:          group + "/" + first.name + "_to_" + second.name + "_" + fractionName(fraction) + "_" + offset.name,
					Group:       group,
					Illuminant:  "ACES_D60",
					AP0:         coords,
					SourceWhite: ACESWhite,
					SourceLab:   LabFromXYZ(xyz, ACESWhite),
				})
			}
		}
	}
	return samples
}

func interpolate(first, second Vec3, fraction float64) Vec3 {
	out := Vec3{}
	for index := range out {
		out[index] = first[index] + (second[index]-first[index])*fraction
	}
	return out
}

func addInteriorOffset(point Vec3, offset float64) Vec3 {
	center := Vec3{1.0 / 3.0, 1.0 / 3.0, 1.0 / 3.0}
	return Vec3{
		point[0] + (center[0]-point[0])*offset,
		point[1] + (center[1]-point[1])*offset,
		point[2] + (center[2]-point[2])*offset,
	}
}

func fractionName(value float64) string {
	switch value {
	case 0:
		return "0"
	case 0.125:
		return "1_8"
	case 0.25:
		return "1_4"
	case 0.5:
		return "1_2"
	case 0.75:
		return "3_4"
	case 0.875:
		return "7_8"
	default:
		return "1"
	}
}
