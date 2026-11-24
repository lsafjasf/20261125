package main

// clusterRegions groups adjacent same-label artifact tiles into hotspots.
func clusterRegions(tiles []TileReport, cols, rows, tileSize int) []Region {
	visited := make([]bool, len(tiles))
	var regions []Region
	for id, t := range tiles {
		if visited[id] || !isArtifact(t.Label) {
			continue
		}
		stack := []int{id}
		visited[id] = true
		region := Region{Label: t.Label,
			X: t.X, Y: t.Y, W: tileSize, H: tileSize}
		minTX, minTY, maxTX, maxTY := t.TX, t.TY, t.TX, t.TY
		severity := 0.0
		for len(stack) > 0 {
			cur := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			ct := tiles[cur]
			region.TileIDs = append(region.TileIDs, cur)
			severity += ct.Score
			if ct.TX < minTX {
				minTX = ct.TX
			}
			if ct.TX > maxTX {
				maxTX = ct.TX
			}
			if ct.TY < minTY {
				minTY = ct.TY
			}
			if ct.TY > maxTY {
				maxTY = ct.TY
			}
			for _, nb := range neighbors(ct.TX, ct.TY, cols, rows) {
				if !visited[nb] && tiles[nb].Label == t.Label {
					visited[nb] = true
					stack = append(stack, nb)
				}
			}
		}
		region.X = minTX * tileSize
		region.Y = minTY * tileSize
		region.W = (maxTX - minTX + 1) * tileSize
		region.H = (maxTY - minTY + 1) * tileSize
		region.Severity = severity / float64(len(region.TileIDs))
		regions = append(regions, region)
	}
	sortRegions(regions)
	return regions
}

func neighbors(tx, ty, cols, rows int) []int {
	var out []int
	if tx > 0 {
		out = append(out, ty*cols+tx-1)
	}
	if tx+1 < cols {
		out = append(out, ty*cols+tx+1)
	}
	if ty > 0 {
		out = append(out, (ty-1)*cols+tx)
	}
	if ty+1 < rows {
		out = append(out, (ty+1)*cols+tx)
	}
	return out
}

func sortRegions(r []Region) {
	for i := 1; i < len(r); i++ {
		for j := i; j > 0 && r[j].Severity*float64(len(r[j].TileIDs)) >
			r[j-1].Severity*float64(len(r[j-1].TileIDs)); j-- {
			r[j], r[j-1] = r[j-1], r[j]
		}
	}
}
