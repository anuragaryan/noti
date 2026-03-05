package logging

import (
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/getsentry/sentry-go"
	"github.com/wailsapp/wails/v2/pkg/logger"
)

// SentryLogger implements the wails logger.Logger interface and routes
// Error and Fatal messages to Sentry. Lower-severity messages are recorded
// as Sentry breadcrumbs so they appear as context when an error is captured.
type SentryLogger struct{}

// NewSentryLogger returns a logger.Logger that forwards Error/Fatal to Sentry.
func NewSentryLogger() logger.Logger { return &SentryLogger{} }

// Print writes the message to stderr only (no Sentry event).
func (s *SentryLogger) Print(msg string) {
	fmt.Fprintln(os.Stderr, "[PRINT]", msg)
}

// Trace records a debug-level breadcrumb.
func (s *SentryLogger) Trace(msg string) {
	s.breadcrumb("debug", msg)
}

// Debug records a debug-level breadcrumb.
func (s *SentryLogger) Debug(msg string) {
	s.breadcrumb("debug", msg)
}

// Info records an info-level breadcrumb.
func (s *SentryLogger) Info(msg string) {
	s.breadcrumb("info", msg)
}

// Warning records a warning-level breadcrumb.
func (s *SentryLogger) Warning(msg string) {
	s.breadcrumb("warning", msg)
}

// Error captures the message as a Sentry event at error level.
func (s *SentryLogger) Error(msg string) {
	fmt.Fprintln(os.Stderr, "[ERROR]", msg)
	sentry.WithScope(func(scope *sentry.Scope) {
		scope.SetLevel(sentry.LevelError)
		sentry.CaptureMessage(msg)
	})
}

// Fatal captures the message as a Sentry exception at fatal level and flushes
// the Sentry client so the event is delivered before the process exits.
func (s *SentryLogger) Fatal(msg string) {
	fmt.Fprintln(os.Stderr, "[FATAL]", msg)
	sentry.WithScope(func(scope *sentry.Scope) {
		scope.SetLevel(sentry.LevelFatal)
		sentry.CaptureException(errors.New(msg))
	})
	sentry.Flush(2 * time.Second)
}

// breadcrumb adds a Sentry breadcrumb and writes the message to stderr.
func (s *SentryLogger) breadcrumb(level, msg string) {
	fmt.Fprintln(os.Stderr, "["+level+"]", msg)
	sentry.AddBreadcrumb(&sentry.Breadcrumb{
		Level:     sentry.Level(level),
		Message:   msg,
		Timestamp: time.Now(),
	})
}
