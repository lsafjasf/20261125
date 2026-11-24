package anc

// DIDInfo describes a registered SMPTE ANC Data Identifier.
type DIDInfo struct {
	DID  byte
	Name string
	// Type1 reports whether bit b7 of the DID is set, i.e. the packet uses a
	// DBN instead of an SDID (SMPTE ST 291-1 §6.1).
	Type1 bool
	SDIDs map[byte]string
}

// didRegistry holds the small set of widely deployed identifiers worth
// labelling. Anything absent is reported as unknown but is never dropped.
var didRegistry = map[byte]DIDInfo{
	0x00: {DID: 0x00, Name: "Reserved / invalid DID"},
	0x41: {
		DID:  0x41,
		Name: "AFD and bar data (SMPTE ST 2016)",
		SDIDs: map[byte]string{
			0x05: "Active Format Description and bar data",
		},
	},
	0x60: {
		DID:  0x60,
		Name: "SMPTE 12M ancillary time code / RP 188",
		SDIDs: map[byte]string{
			0x01: "Time code",
			0x02: "Time code / alternate",
		},
	},
	0x61: {
		DID:  0x61,
		Name: "Digital closed captioning",
		SDIDs: map[byte]string{
			0x01: "CTA-708 caption distribution packet (CDP)",
			0x02: "CTA-608 / legacy caption pair",
		},
	},
}

// RegisterDID adds or replaces a DID label. It is intended for site-specific
// identifier tables; parsing never depends on registration.
func RegisterDID(info DIDInfo) {
	didRegistry[info.DID] = info
}

// LookupDID returns the registered information for a DID and whether it was
// found. Unknown DIDs still get a stable placeholder name from DIDName.
func LookupDID(did byte) (DIDInfo, bool) {
	info, ok := didRegistry[did]
	info.Type1 = did&0x80 != 0
	return info, ok
}

// DIDName returns a human readable label for a DID. Unknown identifiers are
// labelled "Unknown DID 0x.." so they remain visible in reports.
func DIDName(did byte) string {
	if info, ok := didRegistry[did]; ok && info.Name != "" {
		return info.Name
	}
	return "Unknown DID 0x" + twoHex(did)
}

// SDIDName returns the registered sub-label for a DID/SDID pair, if any.
func SDIDName(did, sdid byte) string {
	if info, ok := didRegistry[did]; ok {
		if name, ok := info.SDIDs[sdid]; ok {
			return name
		}
	}
	return ""
}

func twoHex(b byte) string {
	const hexd = "0123456789ABCDEF"
	return string([]byte{hexd[b>>4], hexd[b&0xF]})
}
