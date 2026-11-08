package main

import (
	"encoding/json"
	"os"
	"os/exec"
	"strings"
	"testing"
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
