package matroska

import "fmt"

// ErrorKind classifies parser failures. It implements error so callers can use
// errors.Is(err, matroska.ErrTruncated) and similar checks.
type ErrorKind string

const (
	ErrTruncated         ErrorKind = "truncated"
	ErrInvalidVINT       ErrorKind = "invalid_vint"
	ErrInvalidBlock      ErrorKind = "invalid_block"
	ErrFirstFrameSize    ErrorKind = "first_frame_size"
	ErrNegativeDelta     ErrorKind = "negative_lacing_delta"
	ErrFrameSize         ErrorKind = "frame_size"
	ErrUnknownElement    ErrorKind = "unknown_element"
	ErrBlockBoundary     ErrorKind = "block_boundary"
	ErrUnsupportedLacing ErrorKind = "unsupported_lacing"
	ErrInvalidElement    ErrorKind = "invalid_element"
)

func (k ErrorKind) Error() string { return string(k) }

// ParseError contains the location and classification of a parse failure.
// Kind is always one of the Err* values above. Cause may contain a more
// specific failure, such as a truncated VINT.
type ParseError struct {
	Kind   ErrorKind
	Offset int64
	Frame  int
	Detail string
	Cause  error
}

func (e *ParseError) Error() string {
	where := fmt.Sprintf("offset %d", e.Offset)
	if e.Frame >= 0 {
		where += fmt.Sprintf(", frame %d", e.Frame)
	}
	if e.Detail != "" {
		return fmt.Sprintf("matroska: %s at %s: %s", e.Kind, where, e.Detail)
	}
	return fmt.Sprintf("matroska: %s at %s", e.Kind, where)
}

// Unwrap exposes both the classification and the lower-level cause.
func (e *ParseError) Unwrap() []error {
	errs := []error{e.Kind}
	if e.Cause != nil {
		errs = append(errs, e.Cause)
	}
	return errs
}

func parseError(kind ErrorKind, offset int64, frame int, detail string) *ParseError {
	return &ParseError{Kind: kind, Offset: offset, Frame: frame, Detail: detail}
}
