package ffv1

import "fmt"

var crc32RawTable = func() *[256]uint32 {
	table := &[256]uint32{}
	for i := 0; i < 256; i++ {
		crc := uint32(i) << 24
		for bit := 0; bit < 8; bit++ {
			if crc&0x80000000 != 0 {
				crc = (crc << 1) ^ 0x04C11DB7
			} else {
				crc <<= 1
			}
		}
		table[i] = crc
	}
	return table
}()

type CRCStatus int

const (
	CRCUnknown CRCStatus = iota
	CRCValid
	CRCInvalid
)

type Slice struct {
	Index         int
	Start         int
	End           int
	FooterStart   int
	SizeField     uint32
	ErrorStatus   byte
	CRCEnabled    bool
	StoredCRC     uint32
	CalculatedCRC uint32
	CRCStatus     CRCStatus
}

type Report struct {
	Size           int
	SliceCount     int
	ExpectedSlices int
	SliceCountOK   bool
	HasExpectation bool
	CRCEnabled     bool
	CRCAvailable   bool
	CRCOK          bool
	Slices         []Slice
	UnparsedPrefix int
	ChainOK        bool
	Issues         []string
}

type Mode int

const (
	ModeAuto Mode = iota
	ModeCRC
	ModeNoCRC
)

func CRC32Raw(data []byte) uint32 {
	crc := uint32(0)
	for _, b := range data {
		crc = (crc << 8) ^ crc32RawTable[byte(crc>>24)^b]
	}
	return crc
}

func lastCRCFooterValid(frame []byte) bool {
	if len(frame) < 8 {
		return false
	}
	end := len(frame)
	footerStart := end - 8
	sizeField := uint32(frame[footerStart])<<16 |
		uint32(frame[footerStart+1])<<8 |
		uint32(frame[footerStart+2])
	start := footerStart - int(sizeField)
	if start < 0 || start >= footerStart {
		return false
	}
	return CRC32Raw(frame[start:end]) == 0
}

func AnalyzeConfig(config []byte) (stored, calculated uint32, ok bool) {
	if len(config) < 4 {
		return 0, CRC32Raw(config), false
	}
	stored = uint32(config[len(config)-4])<<24 |
		uint32(config[len(config)-3])<<16 |
		uint32(config[len(config)-2])<<8 |
		uint32(config[len(config)-1])
	calculated = CRC32Raw(config[:len(config)-4])
	whole := CRC32Raw(config)
	return stored, calculated, whole == 0
}

func Analyze(frame []byte, mode Mode, expectedSlices int) Report {
	report := Report{
		Size:           len(frame),
		ExpectedSlices: expectedSlices,
		HasExpectation: expectedSlices > 0,
		ChainOK:        true,
	}
	if len(frame) == 0 {
		report.ChainOK = false
		report.Issues = append(report.Issues, "empty FFV1 sample")
		return report
	}

	useCRC := mode == ModeCRC
	if mode == ModeAuto {
		useCRC = lastCRCFooterValid(frame)
	}
	footerLen := 3
	if useCRC {
		footerLen = 8
		report.CRCEnabled = true
		report.CRCAvailable = true
	}

	end := len(frame)
	for depth := 0; depth < 4096 && end > 0; depth++ {
		if end < footerLen {
			report.ChainOK = false
			report.UnparsedPrefix = end
			report.Issues = append(report.Issues,
				fmt.Sprintf("%d trailing byte(s) without a complete slice footer", end))
			break
		}
		footerStart := end - footerLen
		sizeField := uint32(frame[footerStart])<<16 |
			uint32(frame[footerStart+1])<<8 |
			uint32(frame[footerStart+2])
		start := footerStart - int(sizeField)
		if sizeField == 0 || start < 0 || start >= footerStart {
			report.ChainOK = false
			report.UnparsedPrefix = end
			report.Issues = append(report.Issues,
				fmt.Sprintf("invalid slice_size %d at frame offset %d", sizeField, footerStart))
			break
		}

		slice := Slice{
			Index:       len(report.Slices),
			Start:       start,
			End:         end,
			FooterStart: footerStart,
			SizeField:   sizeField,
			CRCEnabled:  useCRC,
		}
		if useCRC {
			slice.ErrorStatus = frame[footerStart+3]
			slice.StoredCRC = uint32(frame[footerStart+4])<<24 |
				uint32(frame[footerStart+5])<<16 |
				uint32(frame[footerStart+6])<<8 |
				uint32(frame[footerStart+7])
			slice.CalculatedCRC = CRC32Raw(frame[start : end-4])
			if CRC32Raw(frame[start:end]) == 0 {
				slice.CRCStatus = CRCValid
			} else {
				slice.CRCStatus = CRCInvalid
				report.CRCOK = false
			}
		}
		report.Slices = append(report.Slices, slice)
		end = start
		if end == 0 {
			break
		}
	}

	if end > 0 {
		report.UnparsedPrefix = end
		if report.ChainOK {
			report.ChainOK = false
			report.Issues = append(report.Issues, "slice chain did not reach frame start")
		}
	}

	for left, right := 0, len(report.Slices)-1; left < right; left, right = left+1, right-1 {
		report.Slices[left], report.Slices[right] = report.Slices[right], report.Slices[left]
	}
	for i := range report.Slices {
		report.Slices[i].Index = i
	}
	report.SliceCount = len(report.Slices)
	if useCRC {
		allValid := len(report.Slices) > 0
		for _, slice := range report.Slices {
			if slice.CRCStatus != CRCValid {
				allValid = false
				break
			}
		}
		report.CRCOK = allValid
	}
	if expectedSlices > 0 {
		report.SliceCountOK = report.SliceCount == expectedSlices
	}
	return report
}
