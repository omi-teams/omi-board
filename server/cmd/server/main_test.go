package main

import (
	"context"
	"encoding/json"
	"net/url"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/omi-teams/omi-board/server/internal/auth"
	"github.com/omi-teams/omi-board/server/internal/db"
)

func TestFailureLogIsJSON(t *testing.T) {
	if os.Getenv("OBS_TEST_SERVER") == "1" {
		os.Args = []string{"server"}
		os.Exit(mainExit())
	}
	t.Setenv("OBS_TEST_SERVER", "1")
	t.Setenv("SENTRY_DSN", "")
	t.Setenv("PORT", "configuration-secret")
	cmd := exec.Command(os.Args[0], "-test.run=^TestFailureLogIsJSON$")
	output, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatal("expected invalid configuration to fail")
	}
	var entry map[string]interface{}
	if err := json.Unmarshal(output, &entry); err != nil {
		t.Fatalf("expected one JSON error line: %s: %v", output, err)
	}
	if entry["level"] != "ERROR" || entry["msg"] != "server command failed" {
		t.Fatalf("unexpected error log: %v", entry)
	}
	if strings.Contains(string(output), "configuration-secret") {
		t.Fatalf("configuration secret logged: %s", output)
	}
}

func TestCreateUser(t *testing.T) {
	base := os.Getenv("DATABASE_URL")
	if base == "" {
		t.Skip("DATABASE_URL is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	admin, err := db.Open(ctx, base)
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close()
	const schema = "cmd_create_user"
	if _, err := admin.Exec(ctx, "CREATE SCHEMA "+schema); err != nil {
		t.Fatal(err)
	}
	defer func() { _, _ = admin.Exec(context.Background(), "DROP SCHEMA "+schema+" CASCADE") }()
	u, err := url.Parse(base)
	if err != nil {
		t.Fatal(err)
	}
	q := u.Query()
	q.Set("search_path", schema)
	u.RawQuery = q.Encode()
	t.Setenv("DATABASE_URL", u.String())

	if err := createUser("", strings.NewReader("long-enough-password\n")); err == nil {
		t.Fatal("missing email accepted")
	}
	if err := createUser("ann@example.com", strings.NewReader("short\n")); err == nil {
		t.Fatal("short password accepted")
	}
	if err := createUser("ann@example.com", strings.NewReader("long-enough-password\r\n")); err != nil {
		t.Fatal(err)
	}
	if err := createUser("ANN@example.com", strings.NewReader("long-enough-password\n")); err == nil {
		t.Fatal("duplicate email accepted")
	}
	pool, err := db.Open(ctx, u.String())
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	if _, _, err := (auth.Store{DB: pool}).Login(ctx, "ann@example.com", "long-enough-password"); err != nil {
		t.Fatalf("the trailing newline must not be part of the password: %v", err)
	}
}
