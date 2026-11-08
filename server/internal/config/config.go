// Package config reads the server configuration from environment variables.
package config

import (
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strconv"
)

var cookieDomain = regexp.MustCompile(`^[A-Za-z0-9]([A-Za-z0-9.-]*[A-Za-z0-9])?$`)

const minSecretBytes = 32

// Config holds every setting the server reads from the environment.
type Config struct {
	DatabaseURL   string
	Port          string
	PublicWebURL  string
	SessionSecret string
	// CookieDomain is the parent domain of the session cookie (for example example.com); empty means host-only.
	CookieDomain string
}

// Load builds a Config using get to look up variables (os.Getenv in production).
func Load(get func(string) string) (Config, error) {
	c := Config{
		DatabaseURL:   get("DATABASE_URL"),
		Port:          get("PORT"),
		PublicWebURL:  get("PUBLIC_WEB_URL"),
		SessionSecret: get("SESSION_SECRET"),
		CookieDomain:  get("COOKIE_DOMAIN"),
	}
	if c.Port == "" {
		c.Port = "8080"
	}
	if n, err := strconv.Atoi(c.Port); err != nil || n < 1 || n > 65535 {
		return Config{}, fmt.Errorf("PORT %q is not a valid port", c.Port)
	}
	if c.DatabaseURL == "" {
		return Config{}, errors.New("DATABASE_URL is required")
	}
	if c.PublicWebURL != "" {
		if u, err := url.Parse(c.PublicWebURL); err != nil || u.Scheme == "" || u.Host == "" {
			return Config{}, fmt.Errorf("PUBLIC_WEB_URL %q is not an absolute URL", c.PublicWebURL)
		}
	}
	if len(c.SessionSecret) < minSecretBytes {
		return Config{}, fmt.Errorf("SESSION_SECRET must be at least %d bytes", minSecretBytes)
	}
	if c.CookieDomain != "" && !cookieDomain.MatchString(c.CookieDomain) {
		return Config{}, fmt.Errorf("COOKIE_DOMAIN %q is not a bare domain name", c.CookieDomain)
	}
	return c, nil
}
