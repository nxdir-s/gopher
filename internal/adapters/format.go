package adapters

import (
	"go/format"
	"strconv"
	"strings"
)

// ErrInvalidSource reports source that does not parse as go. It annotates the
// message with the offending line so a broken template is easy to locate
type ErrInvalidSource struct {
	src []byte
	err error
}

func (e *ErrInvalidSource) Error() string {
	msg := "generated source is not valid go: " + e.err.Error()

	line, ok := errorLine(e.err)
	if !ok {
		return msg
	}

	lines := strings.Split(string(e.src), "\n")
	if line < 1 || line > len(lines) {
		return msg
	}

	return msg + "\n\t" + strconv.Itoa(line) + " | " + strings.TrimRight(lines[line-1], "\r")
}

// Unwrap returns the underlying formatting error
func (e *ErrInvalidSource) Unwrap() error {
	return e.err
}

type FormatAdapter struct{}

// NewFormatAdapter creates an adapter that formats and validates go source
func NewFormatAdapter() *FormatAdapter {
	return &FormatAdapter{}
}

// Format applies gofmt to the supplied source. A failure here means the template
// produced source that does not parse, so generation should stop
func (a *FormatAdapter) Format(src []byte) ([]byte, error) {
	formatted, err := format.Source(src)
	if err != nil {
		return nil, &ErrInvalidSource{src, err}
	}

	return formatted, nil
}

// errorLine pulls the line number out of a go/format error of the form
// "<line>:<col>: <message>"
func errorLine(err error) (int, bool) {
	msg := err.Error()

	idx := strings.Index(msg, ":")
	if idx < 1 {
		return 0, false
	}

	line, convErr := strconv.Atoi(msg[:idx])
	if convErr != nil {
		return 0, false
	}

	return line, true
}
