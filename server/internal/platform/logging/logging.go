// Package logging builds the application's structured logger (stdlib slog).
package logging

import (
	"log/slog"
	"os"
	"strings"
)

// New returns a JSON slog.Logger at the given level
// (debug | info | warn | error; anything else defaults to info).
//
// It also installs the logger as slog's DEFAULT. Almost everything here is handed
// its logger explicitly, but not everything can be: a dependency that logs through
// the package-level slog functions, and our own fallbacks for a nil logger
// (aibudget.New), would otherwise write plain text at info to stderr — a second
// log format, on a second stream, ignoring LOG_LEVEL. One line makes the whole
// process answer to the same configuration.
//
// Called once, at the top of run(), before anything else can log.
func New(level string) *slog.Logger {
	var lvl slog.Level
	switch strings.ToLower(strings.TrimSpace(level)) {
	case "debug":
		lvl = slog.LevelDebug
	case "warn":
		lvl = slog.LevelWarn
	case "error":
		lvl = slog.LevelError
	default:
		lvl = slog.LevelInfo
	}
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: lvl}))
	slog.SetDefault(logger)
	return logger
}
