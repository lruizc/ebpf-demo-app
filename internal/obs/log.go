// Package obs provides structured logging and request-ID helpers.
package obs

import (
	"crypto/rand"
	"encoding/hex"
	"log/slog"
	"os"
	"strings"
)

// NewLogger builds a JSON slog.Logger writing to stdout at the given level.
func NewLogger(level string) *slog.Logger {
	var l slog.Level
	switch strings.ToLower(level) {
	case "debug":
		l = slog.LevelDebug
	case "warn", "warning":
		l = slog.LevelWarn
	case "error":
		l = slog.LevelError
	default:
		l = slog.LevelInfo
	}
	return slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: l}))
}

// RequestID returns a 16-hex-char random correlation ID.
func RequestID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
