// Package health serves the liveness and database check endpoint.
package health

import (
	"context"
	"net/http"
	"time"
)

const pingTimeout = 2 * time.Second

// Pinger is satisfied by the database pool.
type Pinger interface {
	Ping(ctx context.Context) error
}

// Handler answers 200 when the database responds and 503 otherwise.
func Handler(p Pinger) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), pingTimeout)
		defer cancel()
		if err := p.Ping(ctx); err != nil {
			http.Error(w, "database unavailable", http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = w.Write([]byte("ok\n"))
	})
}
