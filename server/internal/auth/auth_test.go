package auth

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/omi-teams/omi-board/server/internal/db"
	"github.com/omi-teams/omi-board/server/migrations"
)

// testPool returns a pool on a fresh schema with all migrations applied.
func testPool(t *testing.T, ctx context.Context) *pgxpool.Pool {
	t.Helper()
	url := os.Getenv("DATABASE_URL")
	if url == "" {
		t.Skip("DATABASE_URL is not set")
	}
	admin, err := db.Open(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	schema := "auth_" + strings.ToLower(strings.NewReplacer("/", "_", " ", "_").Replace(t.Name()))
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
	return pool
}

func TestPasswordHash(t *testing.T) {
	hash, err := HashPassword("correct horse")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(hash, "$argon2id$v=19$") {
		t.Fatalf("not an argon2id PHC string: %s", hash)
	}
	other, _ := HashPassword("correct horse")
	if hash == other {
		t.Fatal("salt is not random")
	}
	if !VerifyPassword("correct horse", hash) || VerifyPassword("wrong", hash) {
		t.Fatal("verify does not match the password")
	}
	for _, bad := range []string{"", "plain", "$argon2id$v=19$m=1,t=1,p=1$AA$", "$argon2i$v=19$m=19456,t=2,p=1$AA$AA",
		"$argon2id$v=19$m=4294967295,t=2,p=1$AAAA$AAAA"} {
		if VerifyPassword("correct horse", bad) {
			t.Fatalf("malformed hash accepted: %q", bad)
		}
	}
	if !VerifyPassword("omi-board dummy password", dummyHash()) {
		t.Fatal("dummy hash must be a valid argon2id hash")
	}
}

func TestTokens(t *testing.T) {
	a, ah, err := NewToken()
	if err != nil {
		t.Fatal(err)
	}
	b, _, _ := NewToken()
	if a == b || len(ah) != 32 || string(HashToken(a)) != string(ah) {
		t.Fatal("tokens must be random and hash to sha256")
	}
	secret := []byte(strings.Repeat("k", 32))
	if csrfToken(secret, 1) == csrfToken(secret, 2) || csrfToken(secret, 1) != csrfToken(secret, 1) ||
		csrfToken(secret, 1) == csrfToken([]byte(strings.Repeat("j", 32)), 1) {
		t.Fatal("csrf token must depend on session id and secret")
	}
}

func TestStoreLoginAndSessions(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	pool := testPool(t, ctx)
	store := Store{DB: pool}
	user, err := store.CreateUser(ctx, " Ann@Example.com ", "s3cret-password")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.CreateUser(ctx, "ann@example.COM", "another-password"); !errors.Is(err, ErrEmailTaken) {
		t.Fatalf("duplicate email (case-insensitive): %v", err)
	}
	if _, _, err := store.Login(ctx, "ann@example.com", "wrong"); !errors.Is(err, ErrInvalidCredentials) {
		t.Fatal(err)
	}
	if _, _, err := store.Login(ctx, "nobody@example.com", "s3cret-password"); !errors.Is(err, ErrInvalidCredentials) {
		t.Fatal(err)
	}
	token, sess, err := store.Login(ctx, "ANN@example.com", "s3cret-password")
	if err != nil {
		t.Fatal(err)
	}
	if sess.User != user {
		t.Fatalf("user = %+v, want %+v", sess.User, user)
	}
	var stored []byte
	if err := pool.QueryRow(ctx, `SELECT token_hash FROM sessions WHERE id = $1`, sess.ID).Scan(&stored); err != nil || string(stored) != string(HashToken(token)) {
		t.Fatalf("the sha256 of the token must be stored, not the token: %v", err)
	}
	got, err := store.Session(ctx, token)
	if err != nil || got.ID != sess.ID {
		t.Fatalf("session = %+v, %v", got, err)
	}
	if _, err := pool.Exec(ctx, `UPDATE sessions SET expires_at = now() - interval '1 second'`); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Session(ctx, token); !errors.Is(err, ErrNoSession) {
		t.Fatalf("expired session: %v", err)
	}
	if _, _, err := store.Login(ctx, "ann@example.com", "s3cret-password"); err != nil {
		t.Fatal(err)
	}
	var n int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM sessions`).Scan(&n); err != nil || n != 1 {
		t.Fatalf("expired sessions should be purged on login: n=%d err=%v", n, err)
	}
	if err := store.DeleteSession(ctx, sess.ID); err != nil {
		t.Fatal(err)
	}
}

func TestMiddleware(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	pool := testPool(t, ctx)
	a := New(pool, strings.Repeat("s", 32), "https://board.example.com")
	if _, err := a.Store.CreateUser(ctx, "ann@example.com", "s3cret-password"); err != nil {
		t.Fatal(err)
	}
	token, sess, err := a.Store.Login(ctx, "ann@example.com", "s3cret-password")
	if err != nil {
		t.Fatal(err)
	}
	var seen bool
	h := a.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, seen = FromContext(r.Context())
		w.WriteHeader(http.StatusNoContent)
	}))
	do := func(method, path, cookie, csrf, origin string) int {
		seen = false
		req := httptest.NewRequest(method, path, nil)
		if cookie != "" {
			req.AddCookie(&http.Cookie{Name: CookieName, Value: cookie})
		}
		if csrf != "" {
			req.Header.Set(CSRFHeader, csrf)
		}
		if origin != "" {
			req.Header.Set("Origin", origin)
		}
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec.Code
	}
	good := a.CSRFToken(sess.ID)
	for _, tc := range []struct {
		name                       string
		method, path, cookie, csrf string
		origin                     string
		status                     int
		wantSession                bool
	}{
		{"read needs no csrf", "GET", "/v1/x", token, "", "", 204, true},
		{"no cookie write passes through", "POST", "/v1/x", "", "", "", 204, false},
		{"stale cookie write passes through", "POST", "/v1/x", "stale", "", "", 204, false},
		{"write without csrf", "POST", "/v1/x", token, "", "", 403, false},
		{"write with wrong csrf", "DELETE", "/v1/x", token, good + "0", "", 403, false},
		{"write with csrf", "PATCH", "/v1/x", token, good, "", 204, true},
		{"login from the web origin", "POST", loginPath, "", "", "https://board.example.com", 204, false},
		{"login without origin", "POST", loginPath, "", "", "", 204, false},
		{"login from another origin", "POST", loginPath, "", "", "https://evil.example", 403, false},
		{"login with session skips csrf", "POST", loginPath, token, "", "", 204, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := do(tc.method, tc.path, tc.cookie, tc.csrf, tc.origin); got != tc.status || seen != tc.wantSession {
				t.Fatalf("status %d session %v, want %d %v", got, seen, tc.status, tc.wantSession)
			}
		})
	}
}
