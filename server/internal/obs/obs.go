// Package obs provides JSON access logs and optional error reporting.
package obs

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/getsentry/sentry-go"
)

func Logger(out io.Writer) *slog.Logger {
	return slog.New(slog.NewJSONHandler(out, nil))
}

type requestIDKey struct{}

func RequestID(ctx context.Context) string {
	id, _ := ctx.Value(requestIDKey{}).(string)
	return id
}

// NewReporter deliberately bypasses SDK initialization for an empty DSN,
// including its implicit environment-variable fallback.
func NewReporter(dsn string) (*sentry.Client, error) {
	if dsn == "" {
		return nil, nil
	}
	return sentry.NewClient(sentry.ClientOptions{Dsn: dsn, SendDefaultPII: false})
}

func Capture(client *sentry.Client, err error, requestID string) {
	if client == nil {
		return
	}
	for cause := errors.Unwrap(err); cause != nil; cause = errors.Unwrap(err) {
		err = cause
	}
	capture(client, "server error", fmt.Sprintf("%T", err), requestID, nil)
}

// Build events from an allowlist: error and panic values may contain secrets.
func capture(client *sentry.Client, message, typeName, requestID string, tags map[string]string) {
	if client == nil {
		return
	}
	event := sentry.NewEvent()
	event.Level = sentry.LevelError
	event.Message = message
	if typeName != "" {
		event.Exception = []sentry.Exception{{Type: typeName}}
	}
	if tags != nil {
		event.Tags = tags
	}
	if requestID != "" {
		event.Tags["request_id"] = requestID
	}
	client.CaptureEvent(event, nil, sentry.NewScope())
}

// Middleware logs only an allowlist of fields. Bodies, query strings, headers,
// panic values and arbitrary error messages must never be added to access logs.
func Middleware(logger *slog.Logger, client *sentry.Client, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		// Generate locally instead of trusting a potentially sensitive header.
		id := rand.Text()
		w.Header().Set("X-Request-ID", id)
		r = r.WithContext(context.WithValue(r.Context(), requestIDKey{}, id))
		response := &responseWriter{ResponseWriter: w}
		defer func() {
			if recovered := recover(); recovered != nil {
				if recovered == http.ErrAbortHandler {
					panic(recovered)
				}
				capture(client, "handler panic", fmt.Sprintf("%T", recovered), id, nil)
				if response.status == 0 {
					http.Error(response, "Internal Server Error", http.StatusInternalServerError)
				}
			} else if response.status >= 500 {
				capture(client, "HTTP 5xx", "", id, map[string]string{"status": strconv.Itoa(response.status)})
			}
			status := response.status
			if status == 0 {
				status = http.StatusOK
			}
			logger.InfoContext(r.Context(), "request", "method", r.Method,
				"path", r.URL.Path, "status", status,
				"duration", time.Since(start).Seconds(), "request_id", id)
		}()
		next.ServeHTTP(response, r)
	})
}

type responseWriter struct {
	http.ResponseWriter
	status int
}

func (w *responseWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }

func (w *responseWriter) WriteHeader(status int) {
	if w.status != 0 {
		return
	}
	if status >= 200 || status == http.StatusSwitchingProtocols {
		w.status = status
	}
	w.ResponseWriter.WriteHeader(status)
}

func (w *responseWriter) Write(body []byte) (int, error) {
	if w.status == 0 {
		w.WriteHeader(http.StatusOK)
	}
	return w.ResponseWriter.Write(body)
}

func (w *responseWriter) FlushError() error {
	if w.status == 0 {
		w.WriteHeader(http.StatusOK)
	}
	return http.NewResponseController(w.ResponseWriter).Flush()
}
