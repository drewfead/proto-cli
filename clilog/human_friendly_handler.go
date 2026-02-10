package clilog

import (
	"context"
	"fmt"
	"io"
	"log/slog"
)

// HumanFriendlyHandler is a slog.Handler that formats logs in a human-friendly way
// with colorized log levels and no timestamps, ideal for CLI commands.
//
// Thread safety: Handle assembles the complete log line in a local buffer and
// writes it in a single w.Write call, so no mutex is needed. All fields are
// immutable after construction.
type HumanFriendlyHandler struct {
	w     io.Writer
	level slog.Leveler
	attrs []slog.Attr
}

// HumanFriendlySlogHandler creates a new HumanFriendlyHandler that writes to w.
func HumanFriendlySlogHandler(w io.Writer, opts *slog.HandlerOptions) *HumanFriendlyHandler {
	h := &HumanFriendlyHandler{
		w: w,
	}
	if opts != nil {
		h.level = opts.Level
	}
	if h.level == nil {
		h.level = slog.LevelInfo
	}
	return h
}

// Enabled reports whether the handler handles records at the given level.
func (h *HumanFriendlyHandler) Enabled(_ context.Context, level slog.Level) bool {
	return level >= h.level.Level()
}

// Handle formats and writes a log record.
func (h *HumanFriendlyHandler) Handle(_ context.Context, r slog.Record) error {
	// Format: [LEVEL] message key1=value1 key2=value2
	var buf []byte

	// Add colorized level
	buf = append(buf, colorizeLevel(r.Level)...)
	buf = append(buf, ' ')

	// Add message
	buf = append(buf, r.Message...)

	// Add handler attributes
	for _, attr := range h.attrs {
		buf = append(buf, ' ')
		buf = appendAttr(buf, attr)
	}

	// Add record attributes
	r.Attrs(func(a slog.Attr) bool {
		buf = append(buf, ' ')
		buf = appendAttr(buf, a)
		return true
	})

	buf = append(buf, '\n')

	_, err := h.w.Write(buf)
	return err
}

// WithAttrs returns a new handler with the given attributes added.
func (h *HumanFriendlyHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	newAttrs := make([]slog.Attr, len(h.attrs)+len(attrs))
	copy(newAttrs, h.attrs)
	copy(newAttrs[len(h.attrs):], attrs)
	return &HumanFriendlyHandler{
		w:     h.w,
		level: h.level,
		attrs: newAttrs,
	}
}

// WithGroup returns a new handler with the given group added.
// Groups are not supported by HumanFriendlyHandler and this returns the same handler.
func (h *HumanFriendlyHandler) WithGroup(_ string) slog.Handler {
	// Groups don't make much sense for human-readable output
	return h
}

// ANSI color codes
const (
	colorReset  = "\033[0m"
	colorRed    = "\033[31m"
	colorYellow = "\033[33m"
	colorBlue   = "\033[34m"
	colorGray   = "\033[90m"
)

// colorizeLevel returns a colorized string representation of the log level.
func colorizeLevel(level slog.Level) string {
	switch {
	case level >= slog.LevelError:
		return colorRed + "[ERROR]" + colorReset
	case level >= slog.LevelWarn:
		return colorYellow + "[WARN]" + colorReset
	case level >= slog.LevelInfo:
		return colorBlue + "[INFO]" + colorReset
	default:
		return colorGray + "[DEBUG]" + colorReset
	}
}

// appendAttr appends a formatted attribute to the buffer.
func appendAttr(buf []byte, attr slog.Attr) []byte {
	// Handle special cases
	if attr.Equal(slog.Attr{}) {
		return buf
	}

	// Format as key=value
	buf = append(buf, attr.Key...)
	buf = append(buf, '=')
	buf = appendValue(buf, attr.Value)
	return buf
}

// appendValue appends a formatted value to the buffer.
func appendValue(buf []byte, v slog.Value) []byte {
	switch v.Kind() {
	case slog.KindString:
		s := v.String()
		// Quote if contains spaces
		if needsQuoting(s) {
			return append(buf, fmt.Sprintf("%q", s)...)
		}
		return append(buf, s...)
	case slog.KindInt64:
		return append(buf, fmt.Sprintf("%d", v.Int64())...)
	case slog.KindUint64:
		return append(buf, fmt.Sprintf("%d", v.Uint64())...)
	case slog.KindFloat64:
		return append(buf, fmt.Sprintf("%g", v.Float64())...)
	case slog.KindBool:
		return append(buf, fmt.Sprintf("%t", v.Bool())...)
	case slog.KindDuration:
		return append(buf, v.Duration().String()...)
	case slog.KindTime:
		return append(buf, v.Time().Format("15:04:05")...)
	case slog.KindAny:
		return append(buf, fmt.Sprint(v.Any())...)
	case slog.KindGroup:
		// Groups are flattened - just format the value
		return append(buf, fmt.Sprint(v)...)
	case slog.KindLogValuer:
		// Resolve the LogValuer and format the resolved value
		return appendValue(buf, v.Resolve())
	default:
		return append(buf, fmt.Sprint(v)...)
	}
}

// needsQuoting returns true if the string needs to be quoted.
func needsQuoting(s string) bool {
	for _, r := range s {
		if r == ' ' || r == '\t' || r == '\n' || r == '"' {
			return true
		}
	}
	return false
}
