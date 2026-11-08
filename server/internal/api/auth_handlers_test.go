package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/omi-teams/omi-board/server/internal/auth"
	"github.com/omi-teams/omi-board/server/internal/db"
	"github.com/omi-teams/omi-board/server/migrations"
)

const (
	testEmail    = "ann@example.com"
	testPassword = "s3cret-password"
	testOrigin   = "https://board.example.com"
)

// authServer returns the full handler chain (auth middleware + strict handlers) on a fresh schema with one user.
func authServer(t *testing.T) http.Handler {
	t.Helper()
	url := os.Getenv("DATABASE_URL")
	if url == "" {
		t.Skip("DATABASE_URL is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	admin, err := db.Open(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	const schema = "api_auth_handlers"
	if _, err := admin.Exec(ctx, "CREATE SCHEMA "+schema); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = admin.Exec(context.Background(), "DROP SCHEMA "+schema+" CASCADE")
		admin.Close()
	})
	cfg, err := pgxpool.ParseConfig(url)
	if err != nil {
		t.Fatal(err)
	}
	cfg.ConnConfig.RuntimeParams["search_path"] = schema
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	if _, err := db.Migrate(ctx, pool, migrations.FS); err != nil {
		t.Fatal(err)
	}
	a := auth.New(pool, strings.Repeat("s", 32), testOrigin)
	if _, err := a.Store.CreateUser(ctx, testEmail, testPassword); err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	server := Server{DB: fakeDB{}, Auth: a, CookieDomain: "example.com"}
	HandlerFromMux(NewStrictHandlerWithOptions(server, nil, ErrorHandlers()), mux)
	return a.Middleware(mux)
}

func send(h http.Handler, method, path, body string, header map[string]string, cookie *http.Cookie) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	for k, v := range header {
		req.Header.Set(k, v)
	}
	if cookie != nil {
		req.AddCookie(cookie)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func login(h http.Handler, email, password string) *httptest.ResponseRecorder {
	body, _ := json.Marshal(LoginRequest{Email: email, Password: password})
	return send(h, "POST", "/v1/auth/login", string(body), map[string]string{"Content-Type": "application/json", "Origin": testOrigin}, nil)
}

func sessionCookie(t *testing.T, rec *httptest.ResponseRecorder) *http.Cookie {
	t.Helper()
	for _, c := range rec.Result().Cookies() {
		if c.Name == auth.CookieName {
			return c
		}
	}
	t.Fatal("no session cookie in the response")
	return nil
}

func TestLoginSetsCookie(t *testing.T) {
	h := authServer(t)
	rec := login(h, "ANN@example.com", testPassword)
	if rec.Code != 200 {
		t.Fatalf("status %d: %s", rec.Code, rec.Body)
	}
	c := sessionCookie(t, rec)
	if !c.HttpOnly || !c.Secure || c.SameSite != http.SameSiteLaxMode || c.Domain != "example.com" || c.Path != "/" || c.MaxAge <= 0 || c.Value == "" {
		t.Fatalf("cookie attributes: %+v", c)
	}
	var s Session
	if err := json.Unmarshal(rec.Body.Bytes(), &s); err != nil || s.User.Email != testEmail || s.CsrfToken == "" {
		t.Fatalf("body %s: %v", rec.Body, err)
	}
	if strings.Contains(rec.Body.String(), c.Value) {
		t.Fatal("the session token must only be in the cookie")
	}
	again := send(h, "GET", "/v1/auth/session", "", nil, c)
	var s2 Session
	if again.Code != 200 || json.Unmarshal(again.Body.Bytes(), &s2) != nil || s2 != s {
		t.Fatalf("session endpoint: %d %s", again.Code, again.Body)
	}
}

func TestLoginFailuresLookAlike(t *testing.T) {
	h := authServer(t)
	wrong, unknown := login(h, testEmail, "wrong-password"), login(h, "nobody@example.com", "wrong-password")
	for _, rec := range []*httptest.ResponseRecorder{wrong, unknown} {
		if rec.Code != 401 || rec.Header().Get("Content-Type") != "application/problem+json" || len(rec.Result().Cookies()) != 0 {
			t.Fatalf("failure response: %d %v", rec.Code, rec.Header())
		}
	}
	if wrong.Body.String() != unknown.Body.String() {
		t.Fatalf("bodies differ: %s vs %s", wrong.Body, unknown.Body)
	}
	if send(h, "POST", "/v1/auth/login", "{", map[string]string{"Content-Type": "application/json"}, nil).Code != 400 {
		t.Fatal("malformed JSON should be 400")
	}

	// An unknown email must still pay for an argon2id run (the dummy hash).
	best := func(email string) time.Duration {
		d := time.Hour
		for range 3 {
			start := time.Now()
			login(h, email, "wrong-password")
			d = min(d, time.Since(start))
		}
		return d
	}
	known, other := best(testEmail), best("nobody@example.com")
	if other < known/2 {
		t.Fatalf("unknown email answered in %v, known email in %v", other, known)
	}
}

func TestLogoutRevokesSession(t *testing.T) {
	h := authServer(t)
	rec := login(h, testEmail, testPassword)
	c := sessionCookie(t, rec)
	var s Session
	if err := json.Unmarshal(rec.Body.Bytes(), &s); err != nil {
		t.Fatal(err)
	}
	out := send(h, "POST", "/v1/auth/logout", "", map[string]string{auth.CSRFHeader: s.CsrfToken}, c)
	if out.Code != 204 {
		t.Fatalf("logout: %d %s", out.Code, out.Body)
	}
	if cleared := sessionCookie(t, out); cleared.MaxAge >= 0 || cleared.Value != "" {
		t.Fatalf("cookie not cleared: %+v", cleared)
	}
	for _, req := range [][2]string{{"GET", "/v1/auth/session"}, {"POST", "/v1/auth/logout"}} {
		if got := send(h, req[0], req[1], "", map[string]string{auth.CSRFHeader: s.CsrfToken}, c); got.Code != 401 {
			t.Fatalf("%s with the revoked cookie: %d", req[1], got.Code)
		}
	}
}

func TestNoRegistrationRoute(t *testing.T) {
	h := authServer(t)
	for _, method := range []string{"POST", "GET", "PUT"} {
		if got := send(h, method, "/v1/auth/register", `{"email":"a@b.c","password":"x"}`, nil, nil); got.Code != 404 {
			t.Fatalf("%s /v1/auth/register: %d", method, got.Code)
		}
	}
}

func TestCSRFOnCookieWrites(t *testing.T) {
	h := authServer(t)
	rec := login(h, testEmail, testPassword)
	c := sessionCookie(t, rec)
	var s Session
	if err := json.Unmarshal(rec.Body.Bytes(), &s); err != nil {
		t.Fatal(err)
	}
	for name, header := range map[string]map[string]string{
		"missing token": nil,
		"wrong token":   {auth.CSRFHeader: s.CsrfToken + "0"},
		"empty token":   {auth.CSRFHeader: ""},
	} {
		if got := send(h, "POST", "/v1/auth/logout", "", header, c); got.Code != 403 {
			t.Fatalf("%s: %d", name, got.Code)
		}
	}
	if send(h, "GET", "/v1/auth/session", "", nil, c).Code != 200 {
		t.Fatal("a rejected write must not revoke the session")
	}
	// Login is guarded by the Origin check instead.
	body := `{"email":"` + testEmail + `","password":"` + testPassword + `"}`
	if got := send(h, "POST", "/v1/auth/login", body, map[string]string{"Origin": "https://evil.example"}, nil); got.Code != 403 || len(got.Result().Cookies()) != 0 {
		t.Fatalf("login from a foreign origin: %d", got.Code)
	}
}
