package config

import (
	"strings"
	"testing"
)

func env(m map[string]string) func(string) string {
	return func(k string) string { return m[k] }
}

func TestLoad(t *testing.T) {
	secret := strings.Repeat("s", 32)
	tests := []struct {
		name     string
		env      map[string]string
		wantErr  bool
		wantPort string
	}{
		{"defaults port", map[string]string{"DATABASE_URL": "postgres://x", "SESSION_SECRET": secret}, false, "8080"},
		{"custom port", map[string]string{"DATABASE_URL": "postgres://x", "SESSION_SECRET": secret, "PORT": "9000"}, false, "9000"},
		{"missing database url", map[string]string{"SESSION_SECRET": secret}, true, ""},
		{"short secret", map[string]string{"DATABASE_URL": "postgres://x", "SESSION_SECRET": "short"}, true, ""},
		{"bad port", map[string]string{"DATABASE_URL": "postgres://x", "SESSION_SECRET": secret, "PORT": "abc"}, true, ""},
		{"bad web url", map[string]string{"DATABASE_URL": "postgres://x", "SESSION_SECRET": secret, "PUBLIC_WEB_URL": "nope"}, true, ""},
		{"good web url", map[string]string{"DATABASE_URL": "postgres://x", "SESSION_SECRET": secret, "PUBLIC_WEB_URL": "http://localhost:3000"}, false, "8080"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, err := Load(env(tt.env))
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tt.wantErr)
			}
			if err == nil && c.Port != tt.wantPort {
				t.Fatalf("port = %q, want %q", c.Port, tt.wantPort)
			}
		})
	}
}
