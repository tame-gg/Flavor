package logging

import (
	"io"
	"log/slog"
	"os"
)

const Redacted = "[REDACTED]"

func New(w io.Writer, level slog.Level) *slog.Logger {
	if w == nil {
		w = os.Stderr
	}
	return slog.New(slog.NewTextHandler(w, &slog.HandlerOptions{Level: level}))
}

func Default() *slog.Logger {
	return New(os.Stderr, slog.LevelInfo)
}
