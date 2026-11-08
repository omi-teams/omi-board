package auth

import (
	"context"
	"crypto/hmac"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/url"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	// CookieName is the session cookie.
	CookieName = "omi_session"
	// CSRFHeader carries the CSRF token on cookie-authenticated writes.
	CSRFHeader = "X-CSRF-Token"
	loginPath  = "/v1/auth/login"
	// maxLoginBody bounds the password length argon2id has to hash.
	maxLoginBody = 1 << 16
)

// Auth bundles what the middleware and the auth handlers share.
type Auth struct {
	Store     Store
	secret    []byte
	webOrigin string
}

// New builds an Auth. publicWebURL is the web origin allowed to call login ("" allows no browser origin).
func New(pool *pgxpool.Pool, sessionSecret, publicWebURL string) *Auth {
	a := &Auth{Store: Store{DB: pool}, secret: []byte(sessionSecret)}
	if u, err := url.Parse(publicWebURL); err == nil && u.Scheme != "" && u.Host != "" {
		a.webOrigin = strings.ToLower(u.Scheme + "://" + u.Host)
	}
	return a
}

// CSRFToken returns the CSRF token the client must echo in X-CSRF-Token for a session.
func (a *Auth) CSRFToken(sessionID int64) string { return csrfToken(a.secret, sessionID) }

type ctxKey struct{}

// FromContext returns the session the middleware attached, if the request had a valid session cookie.
func FromContext(ctx context.Context) (Session, bool) {
	s, ok := ctx.Value(ctxKey{}).(Session)
	return s, ok
}

// Middleware attaches the cookie's session to the request context. A cookie-authenticated
// POST/PUT/PATCH/DELETE needs a matching X-CSRF-Token (403 otherwise); login, which has no
// session yet, instead requires any Origin header to be the web origin. A missing or
// stale cookie only leaves the context empty: handlers answer 401.
func (a *Auth) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == loginPath {
			if origin := r.Header.Get("Origin"); origin != "" && !strings.EqualFold(origin, a.webOrigin) {
				writeProblem(w, http.StatusForbidden, "Origin not allowed", "origin_not_allowed")
				return
			}
			r.Body = http.MaxBytesReader(w, r.Body, maxLoginBody)
			next.ServeHTTP(w, r)
			return
		}
		if c, err := r.Cookie(CookieName); err == nil {
			sess, err := a.Store.Session(r.Context(), c.Value)
			switch {
			case err == nil:
				if isWrite(r.Method) && !a.csrfValid(r, sess.ID) {
					writeProblem(w, http.StatusForbidden, "CSRF token missing or invalid", "csrf_invalid")
					return
				}
				r = r.WithContext(context.WithValue(r.Context(), ctxKey{}, sess))
			case !errors.Is(err, ErrNoSession):
				slog.ErrorContext(r.Context(), "session lookup failed")
				writeProblem(w, http.StatusInternalServerError, "Internal error", "internal_error")
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

func isWrite(method string) bool {
	switch method {
	case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
		return true
	}
	return false
}

func (a *Auth) csrfValid(r *http.Request, sessionID int64) bool {
	got := r.Header.Get(CSRFHeader)
	return got != "" && hmac.Equal([]byte(got), []byte(a.CSRFToken(sessionID)))
}

func writeProblem(w http.ResponseWriter, status int, title, code string) {
	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{"type": "about:blank", "title": title, "status": status, "code": code})
}
