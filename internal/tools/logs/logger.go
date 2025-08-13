// Package logs constructs the application-wide structured logger.
package logs

import (
	"context"
	"log/slog"
	"os"
)

// New returns a JSON-format slog.Logger writing to stdout.
//
// By default (verbose=false) only Info and Error records are emitted; Warn and
// Debug are suppressed. With verbose=true Warn records are included as well.
// Timestamps are stripped so output stays clean in CI pipelines.
func New(verbose bool) *slog.Logger {
	base := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
		ReplaceAttr: func(_ []string, a slog.Attr) slog.Attr {
			if a.Key == slog.TimeKey {
				return slog.Attr{}
			}

			return a
		},
	})

	return slog.New(&verbosityHandler{Handler: base, verbose: verbose})
}

// verbosityHandler wraps a slog.Handler and suppresses Warn records when
// verbose is false, keeping the default output focused on actionable messages.
type verbosityHandler struct {
	slog.Handler
	verbose bool
}

// Enabled reports whether the given level should produce a log record. Warn is
// only enabled when the handler was created with verbose=true.
func (h *verbosityHandler) Enabled(ctx context.Context, level slog.Level) bool {
	if level == slog.LevelWarn && !h.verbose {
		return false
	}

	return h.Handler.Enabled(ctx, level)
}

// WithAttrs returns a new handler with the given attributes pre-set, preserving
// the verbose setting.
func (h *verbosityHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &verbosityHandler{Handler: h.Handler.WithAttrs(attrs), verbose: h.verbose}
}

// WithGroup returns a new handler scoped to a named group, preserving the
// verbose setting.
func (h *verbosityHandler) WithGroup(name string) slog.Handler {
	return &verbosityHandler{Handler: h.Handler.WithGroup(name), verbose: h.verbose}
}
