package lut3d

import (
	"errors"
	"fmt"
)

// ErrorKind classifies a parse failure without depending on message text.
type ErrorKind string

const (
	ErrSyntax      ErrorKind = "syntax"
	ErrHeader      ErrorKind = "header"
	ErrDomain      ErrorKind = "domain"
	ErrEntryCount  ErrorKind = "entry_count"
	ErrUnsupported ErrorKind = "unsupported"
)

// ParseError always carries the 1-based source line at which parsing failed.
// At end of input, Line is one past the final physical line.
type ParseError struct {
	Line    int
	Kind    ErrorKind
	Message string
	Err     error
}

func (e *ParseError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("line %d: %s: %v", e.Line, e.Message, e.Err)
	}
	return fmt.Sprintf("line %d: %s", e.Line, e.Message)
}

func (e *ParseError) Unwrap() error {
	return e.Err
}

// IsParseError reports whether err contains a ParseError.
func IsParseError(err error) bool {
	var parseErr *ParseError
	return errors.As(err, &parseErr)
}
