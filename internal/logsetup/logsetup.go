// Package logsetup configures the default slog logger.
package logsetup

import (
	"fmt"
	"log/slog"
	"os"
)

// Init parses levelStr (e.g. "debug", "info", "warn", "error") and sets it as
// the minimum level for the default slog logger. Returns an error for unknown levels.
func Init(levelStr string) error {
	var level slog.Level
	if err := level.UnmarshalText([]byte(levelStr)); err != nil {
		return fmt.Errorf("invalid log level %q: %w", levelStr, err)
	}
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: level,
	})))
	return nil
}
