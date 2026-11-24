package fcpxml

import (
	"cutcheck/internal/rational"
	"encoding/xml"
	"fmt"
	"strconv"
	"strings"
)

// Event is one asset-clip re-opened from an FCPXML file.
type Event struct {
	ID        string
	Clip      string
	Layer     string // "video" or "audio"
	Track     int
	RecordIn  rational.Rat
	RecordOut rational.Rat
	SourceIn  rational.Rat
	SourceOut rational.Rat
}

type xmlAssetClip struct {
	Name        string `xml:"name,attr"`
	Note        string `xml:"note,attr"`
	Role        string `xml:"role,attr"`
	Offset      string `xml:"offset,attr"`
	Duration    string `xml:"duration,attr"`
	Start       string `xml:"start,attr"`
	SrcDuration string `xml:"srcDuration,attr"`
	Lane        string `xml:"lane,attr"`
}

type xmlSpine struct {
	Clips []xmlAssetClip `xml:"asset-clip"`
}

type xmlSequence struct {
	Spine xmlSpine `xml:"spine"`
}

type xmlProject struct {
	Sequence xmlSequence `xml:"sequence"`
}

type xmlEvent struct {
	Project xmlProject `xml:"project"`
}

type xmlLibrary struct {
	Event xmlEvent `xml:"event"`
}

type xmlFCPXML struct {
	Library xmlLibrary `xml:"library"`
}

// parseRat parses FCPXML time values: "1001/300s", "5s", "0s".
func parseRat(s string) (rational.Rat, error) {
	s = strings.TrimSuffix(strings.TrimSpace(s), "s")
	if s == "" {
		return rational.Zero(), fmt.Errorf("empty time value")
	}
	if i := strings.Index(s, "/"); i >= 0 {
		num, err1 := strconv.ParseInt(s[:i], 10, 64)
		den, err2 := strconv.ParseInt(s[i+1:], 10, 64)
		if err1 != nil || err2 != nil {
			return rational.Zero(), fmt.Errorf("bad rational time %q", s)
		}
		return rational.New(num, den), nil
	}
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return rational.Zero(), fmt.Errorf("bad time value %q", s)
	}
	return rational.FromInt(n), nil
}

// Parse re-opens an FCPXML file and returns all spine clips.
func Parse(text string) ([]Event, error) {
	var doc xmlFCPXML
	if err := xml.Unmarshal([]byte(text), &doc); err != nil {
		return nil, fmt.Errorf("fcpxml: %w", err)
	}
	clips := doc.Library.Event.Project.Sequence.Spine.Clips
	if len(clips) == 0 {
		return nil, fmt.Errorf("fcpxml: no asset-clips found")
	}
	var events []Event
	for i, c := range clips {
		offset, err := parseRat(c.Offset)
		if err != nil {
			return nil, fmt.Errorf("clip %d offset: %w", i, err)
		}
		dur, err := parseRat(c.Duration)
		if err != nil {
			return nil, fmt.Errorf("clip %d duration: %w", i, err)
		}
		start, err := parseRat(c.Start)
		if err != nil {
			return nil, fmt.Errorf("clip %d start: %w", i, err)
		}
		srcDur := dur
		if c.SrcDuration != "" {
			if srcDur, err = parseRat(c.SrcDuration); err != nil {
				return nil, fmt.Errorf("clip %d srcDuration: %w", i, err)
			}
		}
		ev := Event{
			ID:        c.Note,
			Clip:      c.Name,
			Layer:     "video",
			RecordIn:  offset,
			RecordOut: offset.Add(dur),
			SourceIn:  start,
			SourceOut: start.Add(srcDur),
		}
		if c.Role == "audio" {
			ev.Layer = "audio"
		}
		lane := strings.TrimPrefix(c.Lane, "a")
		if v, err := strconv.Atoi(lane); err == nil {
			if ev.Layer == "audio" {
				ev.Track = v
			} else {
				ev.Track = v + 1
			}
		}
		events = append(events, ev)
	}
	return events, nil
}
