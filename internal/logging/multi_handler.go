package logging

import (
	"context"
	"errors"
	"log/slog"
)

// MultiHandler fans out log records to multiple slog.Handler implementations.
// All enabled handlers receive every record.
type MultiHandler struct {
	handlers []slog.Handler
}

// NewMultiHandler returns a [slog.Handler] that writes each record to all
// provided handlers. If a handler's Enabled method returns false the record
// is still forwarded; each handler makes its own filtering decision.
func NewMultiHandler(handlers ...slog.Handler) slog.Handler {
	return &MultiHandler{handlers: handlers}
}

// Enabled reports true if any of the underlying handlers is enabled for the
// given level.
func (m *MultiHandler) Enabled(ctx context.Context, level slog.Level) bool {
	for _, h := range m.handlers {
		if h.Enabled(ctx, level) {
			return true
		}
	}
	return false
}

// Handle passes the record to every underlying handler.
func (m *MultiHandler) Handle(ctx context.Context, r slog.Record) error {
	var errs []error
	for _, h := range m.handlers {
		if h.Enabled(ctx, r.Level) {
			if err := h.Handle(ctx, r.Clone()); err != nil {
				errs = append(errs, err)
			}
		}
	}
	if len(errs) > 0 {
		return errors.Join(errs...)
	}
	return nil
}

// WithAttrs returns a new MultiHandler whose underlying handlers all have the
// given attributes pre-set.
func (m *MultiHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	handlers := make([]slog.Handler, len(m.handlers))
	for i, h := range m.handlers {
		handlers[i] = h.WithAttrs(attrs)
	}
	return &MultiHandler{handlers: handlers}
}

// WithGroup returns a new MultiHandler whose underlying handlers all use the
// given group name.
func (m *MultiHandler) WithGroup(name string) slog.Handler {
	handlers := make([]slog.Handler, len(m.handlers))
	for i, h := range m.handlers {
		handlers[i] = h.WithGroup(name)
	}
	return &MultiHandler{handlers: handlers}
}
