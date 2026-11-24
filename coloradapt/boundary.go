package coloradapt

func encodingCubeSamples() []Sample {
	values := []float64{0, 0.5, 1}
	var samples []Sample
	for _, r := range values {
		for _, g := range values {
			for _, b := range values {
				coords := Vec3{r, g, b}
				halfCoordinates := countHalfCoordinates(coords)
				group := "encoding_cube_face"
				if halfCoordinates == 0 {
					group = "encoding_cube_vertex"
				} else if halfCoordinates == 1 {
					group = "encoding_cube_edge"
				}
				if halfCoordinates == 3 {
					continue
				}
				xyz := XYZFromAP0Sample(coords)
				samples = append(samples, Sample{
					ID:          group + "/" + coordinateName(r) + "_" + coordinateName(g) + "_" + coordinateName(b),
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

func countHalfCoordinates(coords Vec3) int {
	count := 0
	for _, value := range coords {
		if value == 0.5 {
			count++
		}
	}
	return count
}

func coordinateName(value float64) string {
	switch value {
	case 0:
		return "0"
	case 0.5:
		return "half"
	default:
		return "1"
	}
}
