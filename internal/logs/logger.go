package logs

import (
	"io"
	"log/slog"
)

const DebugEnv string = "GOPHER_DEBUG"

// NewLogger creates the logger used across gopher. Output goes to stderr so it
// never mixes with generated source on stdout
func NewLogger(w io.Writer, debug bool) *slog.Logger {
	level := slog.LevelWarn
	if debug {
		level = slog.LevelDebug
	}

	return slog.New(slog.NewTextHandler(w, &slog.HandlerOptions{
		Level: level,
	}))
}
