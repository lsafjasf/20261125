package icc

func (p *Profile) parseTagData(r reader, tag *Tag) {
	if tag.Size <= 0 || tag.Offset < headerSize || tag.Offset+tag.Size > r.len() {
		// Already reported in table validation; nothing safe to read.
		return
	}

	typ, ok := r.sig(tag.Offset)
	if !ok {
		tag.Issues = append(tag.Issues, "cannot read type signature")
		return
	}
	tag.Type = typ

	switch typ {
	case "XYZ ":
		p.parseXYZ(r, tag)
	case "curv":
		p.parseCurv(r, tag)
	case "para":
		p.parsePara(r, tag)
	default:
		tag.Issues = append(tag.Issues, "unsupported type signature "+typ+" for matrix/TRC tag")
		p.addIssue("tag %q: unsupported type %q", tag.Signature, typ)
	}
}

func (p *Profile) parseXYZ(r reader, tag *Tag) {
	// XYZType: 4-byte type + 4 reserved + 3 * s15Fixed16 = 20 bytes.
	if tag.Size != 20 {
		tag.Issues = append(tag.Issues, "wrong length for XYZ type: expected 20, got "+itoa(tag.Size))
		p.addIssue("tag %q: XYZ length %d is not 20", tag.Signature, tag.Size)
		return
	}
	x, _ := r.u32(tag.Offset + 8)
	y, _ := r.u32(tag.Offset + 12)
	z, _ := r.u32(tag.Offset + 16)
	tag.XYZ = &XYZ{fromS15Fixed16(x), fromS15Fixed16(y), fromS15Fixed16(z)}
	tag.Loaded = true
}

func (p *Profile) parseCurv(r reader, tag *Tag) {
	// curveType: 4 type + 4 reserved + 4 count + count*2 entry bytes.
	if tag.Size < 12 {
		tag.Issues = append(tag.Issues, "wrong length: curveType needs at least 12 bytes, got "+itoa(tag.Size))
		p.addIssue("tag %q: curv length %d below minimum", tag.Signature, tag.Size)
		return
	}
	count, _ := r.u32(tag.Offset + 8)
	need := 12 + int(count)*2
	if tag.Size != need {
		tag.Issues = append(tag.Issues,
			"wrong length: curveType count "+itoa(int(count))+" needs "+itoa(need)+" bytes, got "+itoa(tag.Size))
		p.addIssue("tag %q: curv declared %d entries but size is %d (expected %d)",
			tag.Signature, count, tag.Size, need)
		return
	}

	trc := &TRC{}
	switch {
	case count == 0:
		trc.Kind = TRCIdentity
	case count == 1:
		g, _ := r.u16(tag.Offset + 12)
		trc.Kind = TRCGamma
		trc.Gamma = fromU8Fixed8(g)
		if trc.Gamma <= 0 {
			tag.Issues = append(tag.Issues, "illegal curve: gamma must be positive")
			p.addIssue("tag %q: non-positive gamma %g", tag.Signature, trc.Gamma)
			return
		}
	default:
		trc.Kind = TRCLUT
		trc.Entries = make([]float64, count)
		for i := uint32(0); i < count; i++ {
			v, _ := r.u16(tag.Offset + 12 + int(i)*2)
			trc.Entries[i] = fromU16Fixed16(v)
		}
		if err := validateLUT(trc.Entries); err != "" {
			tag.Issues = append(tag.Issues, "illegal curve: "+err)
			p.addIssue("tag %q: %s", tag.Signature, err)
			return
		}
	}
	tag.TRC = trc
	tag.Loaded = true
}

func validateLUT(e []float64) string {
	if e[0] < 0 || e[len(e)-1] > 1 {
		return "entries leave the [0,1] range"
	}
	for i := 1; i < len(e); i++ {
		if e[i] < e[i-1] {
			return "LUT is not monotonic non-decreasing"
		}
	}
	return ""
}

// paraParamCount is the required parameter count for parametricCurveType
// function types 0..4, including the leading gamma.
// type 0: g; 1: g,a,b; 2: g,a,b,c; 3: g,a,b,c,d; 4: g,a,b,c,d,e,f.
var paraParamCount = map[int]int{0: 1, 1: 3, 2: 4, 3: 5, 4: 7}

func (p *Profile) parsePara(r reader, tag *Tag) {
	// parametricCurveType: 4 type + 4 reserved + 2 function type + 2 reserved
	// + n * s15Fixed16.
	if tag.Size < 12 {
		tag.Issues = append(tag.Issues, "wrong length: parametricCurveType needs at least 12 bytes, got "+itoa(tag.Size))
		p.addIssue("tag %q: para length %d below minimum", tag.Signature, tag.Size)
		return
	}
	ft, _ := r.u16(tag.Offset + 8)
	want, known := paraParamCount[int(ft)]
	if !known {
		tag.Issues = append(tag.Issues, "illegal curve: unsupported parametric function type "+itoa(int(ft)))
		p.addIssue("tag %q: para function type %d outside supported 0..4", tag.Signature, ft)
		return
	}
	need := 12 + want*4
	if tag.Size != need {
		tag.Issues = append(tag.Issues,
			"wrong length: para type "+itoa(int(ft))+" needs "+itoa(need)+" bytes, got "+itoa(tag.Size))
		p.addIssue("tag %q: para size %d does not match function type %d (expected %d)",
			tag.Signature, tag.Size, ft, need)
		return
	}

	trc := &TRC{ParaType: int(ft), Params: make([]float64, want)}
	for i := 0; i < want; i++ {
		v, _ := r.u32(tag.Offset + 12 + i*4)
		trc.Params[i] = fromS15Fixed16(v)
	}
	if trc.Params[0] <= 0 {
		tag.Issues = append(tag.Issues, "illegal curve: gamma must be positive")
		p.addIssue("tag %q: non-positive para gamma %g", tag.Signature, trc.Params[0])
		return
	}
	if ft == 0 {
		trc.Kind = TRCGamma
		trc.Gamma = trc.Params[0]
	} else {
		trc.Kind = TRCPara
	}
	tag.TRC = trc
	tag.Loaded = true
}
