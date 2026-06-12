// Package logs constructs the application-wide structured logger.
package logs

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"
	"sync"
	"unicode"

	"github.com/fatih/color"
)

// Format selects the output encoding for the logger.
type Format string

const (
	// FormatText emits colored, human-readable lines (default).
	FormatText Format = "text"
	// FormatJSON emits structured JSON records.
	FormatJSON Format = "json"
)

// Option configures a logger returned by New.
type Option func(*options)

type options struct {
	verbose bool
	format  Format
}

// WithVerbose enables or disables Warn-level records (suppressed by default).
func WithVerbose(v bool) Option {
	return func(o *options) { o.verbose = v }
}

// WithFormat sets the output format.
func WithFormat(f Format) Option {
	return func(o *options) { o.format = f }
}

// New returns a slog.Logger configured by opts.
// Default: colored plain text, Info and Error only, no timestamps.
// Debug is always suppressed; Warn requires WithVerbose(true).
func New(opts ...Option) *slog.Logger {
	o := &options{format: FormatText}
	for _, opt := range opts {
		opt(o)
	}

	if o.format == FormatJSON {
		base := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
			Level: slog.LevelDebug,
			ReplaceAttr: func(_ []string, a slog.Attr) slog.Attr {
				if a.Key == slog.TimeKey {
					return slog.Attr{}
				}

				return a
			},
		})

		return slog.New(&verbosityHandler{Handler: base, verbose: o.verbose})
	}

	return slog.New(newTextHandler(os.Stdout, o.verbose))
}

// level glyphs and colors for the text format.
var (
	infoColor  = color.New(color.FgCyan)
	warnColor  = color.New(color.FgYellow)
	errorColor = color.New(color.FgRed)
	debugColor = color.New(color.FgHiBlack)
	attrColor  = color.New(color.FgHiBlack)
)

const (
	glyphInfo  = "●"
	glyphWarn  = "⚠"
	glyphError = "✗"
	glyphDebug = "·"
)

// textHandler is a slog.Handler that writes colored, human-readable lines.
type textHandler struct {
	mu      *sync.Mutex
	w       io.Writer
	verbose bool
	attrs   []slog.Attr
}

// newTextHandler creates a text handler writing to w.
func newTextHandler(w io.Writer, verbose bool) *textHandler {
	return &textHandler{
		mu:      &sync.Mutex{},
		w:       w,
		verbose: verbose,
	}
}

// Enabled reports whether the given level should produce output.
// Debug and Warn require verbose mode; Info and Error are always shown.
func (h *textHandler) Enabled(_ context.Context, level slog.Level) bool {
	if !h.verbose && (level == slog.LevelDebug || level == slog.LevelWarn) {
		return false
	}

	return level >= slog.LevelDebug
}

// Handle formats r as a colored text line and writes it to h.w.
func (h *textHandler) Handle(_ context.Context, r slog.Record) error {
	var buf bytes.Buffer

	glyph, col := levelStyle(r.Level)
	fmt.Fprintf(&buf, "%s", col.Sprintf("%s %s", glyph, capitalize(r.Message)))

	var parts []string
	for _, a := range h.attrs {
		parts = append(parts, fmt.Sprintf("%s=%v", a.Key, a.Value))
	}

	r.Attrs(func(a slog.Attr) bool {
		parts = append(parts, fmt.Sprintf("%s=%v", a.Key, a.Value))
		return true
	})

	if len(parts) > 0 {
		fmt.Fprintf(&buf, "  %s", attrColor.Sprint(strings.Join(parts, "  ")))
	}

	buf.WriteByte('\n')

	h.mu.Lock()
	defer h.mu.Unlock()

	_, err := h.w.Write(buf.Bytes())

	return err
}

// WithAttrs returns a new handler with attrs appended, sharing the writer lock.
func (h *textHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	combined := make([]slog.Attr, len(h.attrs)+len(attrs))
	copy(combined, h.attrs)
	copy(combined[len(h.attrs):], attrs)

	return &textHandler{
		mu:      h.mu,
		w:       h.w,
		verbose: h.verbose,
		attrs:   combined,
	}
}

// WithGroup returns h unchanged; groups are not used in this codebase.
func (h *textHandler) WithGroup(_ string) slog.Handler { return h }

// levelStyle returns the glyph and color for a given log level.
func levelStyle(level slog.Level) (glyph string, col *color.Color) {
	switch level {
	case slog.LevelWarn:
		return glyphWarn, warnColor
	case slog.LevelError:
		return glyphError, errorColor
	case slog.LevelDebug:
		return glyphDebug, debugColor
	default:
		return glyphInfo, infoColor
	}
}

// capitalize upper-cases the first rune of s.
func capitalize(s string) string {
	if s == "" {
		return s
	}

	r := []rune(s)
	r[0] = unicode.ToUpper(r[0])

	return string(r)
}

// verbosityHandler wraps a slog.Handler and suppresses Warn records when verbose is false.
// Used only for the JSON format path.
type verbosityHandler struct {
	slog.Handler
	verbose bool
}

// Enabled reports whether the given level should produce a log record.
// Debug and Warn are only enabled when verbose is true.
func (h *verbosityHandler) Enabled(ctx context.Context, level slog.Level) bool {
	if !h.verbose && (level == slog.LevelDebug || level == slog.LevelWarn) {
		return false
	}

	return h.Handler.Enabled(ctx, level)
}

// WithAttrs returns a new handler with the given attributes pre-set, preserving verbose.
func (h *verbosityHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &verbosityHandler{Handler: h.Handler.WithAttrs(attrs), verbose: h.verbose}
}

// WithGroup returns a new handler scoped to a named group, preserving verbose.
func (h *verbosityHandler) WithGroup(name string) slog.Handler {
	return &verbosityHandler{Handler: h.Handler.WithGroup(name), verbose: h.verbose}
}
