// Package obs provides JSON access logs and optional error reporting.
package obs

import (
	"context"
	"crypto/rand"
	"fmt"
	"io"
	"log/slog"
	"net/http"
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
	scope := sentry.NewScope()
	if requestID != "" {
		scope.SetTag("request_id", requestID)
	}
	client.CaptureException(err, nil, scope)
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
				Capture(client, fmt.Errorf("handler panic: %v", recovered), id)
				if response.status == 0 {
					http.Error(response, "Internal Server Error", http.StatusInternalServerError)
				}
			} else if response.status >= 500 {
				Capture(client, fmt.Errorf("HTTP %d", response.status), id)
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
