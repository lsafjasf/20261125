package cmx

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"cutcheck/internal/rational"
	"cutcheck/internal/timecode"
)

// Event is one EDL row re-opened from a CMX file. Times are exact seconds.
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

var eventLine = regexp.MustCompile(
	`^(\d{3})\s+(\S+)\s+(V|A\d*|AA)\s+(C|D)\s*(\d*)\s+(\S+)\s+(\S+)\s+(\S+)\s+(\S+)\s*$`)

// Parse re-opens a CMX EDL. fpsNum/fpsDen declare the frame rate the file's
// timecodes are interpreted at; drop-frame is taken from the FCM header.
func Parse(text string, fpsNum, fpsDen int64) ([]Event, error) {
	var events []Event
	var cur *Event
	fps := rational.New(fpsNum, fpsDen)
	nominal := fps.RoundInt64()

	finish := func() {
		if cur != nil {
			events = append(events, *cur)
			cur = nil
		}
	}

	tc := func(s string) (rational.Rat, error) {
		frames, err := timecode.Parse(s, nominal)
		if err != nil {
			return rational.Zero(), err
		}
		return rational.FromInt(frames).Div(fps), nil
	}

	for lineno, raw := range strings.Split(text, "\n") {
		line := strings.TrimRight(raw, " \t\r")
		switch {
		case eventLine.MatchString(line):
			finish()
			m := eventLine.FindStringSubmatch(line)
			cur = &Event{}
			switch {
			case m[3] == "V":
				cur.Layer = "video"
				cur.Track = 1
			default:
				cur.Layer = "audio"
				tr := strings.TrimPrefix(m[3], "A")
				if tr == "" || tr == "A" {
					cur.Track = 1
				} else if v, err := strconv.Atoi(tr); err == nil {
					cur.Track = v
				}
			}
			var err error
			if cur.SourceIn, err = tc(m[6]); err != nil {
				return nil, fmt.Errorf("line %d: %w", lineno+1, err)
			}
			if cur.SourceOut, err = tc(m[7]); err != nil {
				return nil, fmt.Errorf("line %d: %w", lineno+1, err)
			}
			if cur.RecordIn, err = tc(m[8]); err != nil {
				return nil, fmt.Errorf("line %d: %w", lineno+1, err)
			}
			if cur.RecordOut, err = tc(m[9]); err != nil {
				return nil, fmt.Errorf("line %d: %w", lineno+1, err)
			}
		case strings.HasPrefix(line, "* FROM CLIP NAME:") && cur != nil:
			cur.Clip = strings.TrimSpace(strings.TrimPrefix(line, "* FROM CLIP NAME:"))
		case strings.HasPrefix(line, "* EVENT ID:") && cur != nil:
			cur.ID = strings.TrimSpace(strings.TrimPrefix(line, "* EVENT ID:"))
		case strings.HasPrefix(line, "* TRACK: V") && cur != nil:
			if v, err := strconv.Atoi(strings.TrimPrefix(line, "* TRACK: V")); err == nil {
				cur.Track = v
			}
		}
	}
	finish()
	if len(events) == 0 {
		return nil, fmt.Errorf("cmx: no events found")
	}
	return events, nil
}
