package obs

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/getsentry/sentry-go"
)

func TestAccessLog(t *testing.T) {
	for _, tc := range []struct {
		name   string
		serve  func(http.ResponseWriter)
		status int
	}{
		{"empty", func(http.ResponseWriter) {}, 200},
		{"write", func(w http.ResponseWriter) { _, _ = w.Write([]byte("ok")) }, 200},
		{"explicit", func(w http.ResponseWriter) { w.WriteHeader(201); w.WriteHeader(500) }, 201},
		{"failure", func(w http.ResponseWriter) { w.WriteHeader(503) }, 503},
		{"panic", func(http.ResponseWriter) { panic("panic-secret") }, 500},
		{"flush", func(w http.ResponseWriter) { _ = http.NewResponseController(w).Flush() }, 200},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var out bytes.Buffer
			var contextID string
			h := Middleware(Logger(&out), nil, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				contextID = RequestID(r.Context())
				tc.serve(w)
			}))
			r := httptest.NewRequest("POST", "/v1/healthz?token=query-secret", strings.NewReader("body-secret"))
			r.Header.Set("Authorization", "Bearer auth-secret")
			r.Header.Set("Cookie", "session=cookie-secret")
			r.Header.Set("X-API-Key", "header-secret")
			r.Header.Set("X-Request-ID", "untrusted-secret")
			w := httptest.NewRecorder()
			h.ServeHTTP(w, r)
			var entry map[string]interface{}
			decoder := json.NewDecoder(&out)
			// Preserve the raw line for the privacy assertion before decoding consumes it.
			line := out.String()
			if err := decoder.Decode(&entry); err != nil {
				t.Fatal(err)
			}
			if err := decoder.Decode(&map[string]interface{}{}); err != io.EOF {
				t.Fatalf("expected exactly one JSON line: %v", err)
			}
			if entry["method"] != "POST" || entry["path"] != "/v1/healthz" || entry["status"] != float64(tc.status) {
				t.Fatalf("wrong access fields: %v", entry)
			}
			if duration, ok := entry["duration"].(float64); !ok || duration < 0 {
				t.Fatalf("invalid duration: %v", entry["duration"])
			}
			id := w.Header().Get("X-Request-ID")
			if id == "" || id != contextID || entry["request_id"] != id {
				t.Fatalf("request ID mismatch: %v / %q / %q", entry, id, contextID)
			}
			for _, secret := range []string{"secret", "Authorization", "Cookie", "Bearer", "token="} {
				if strings.Contains(line, secret) {
					t.Fatalf("sensitive data logged: %s", line)
				}
			}
			if w.Code != tc.status {
				t.Fatalf("response status = %d", w.Code)
			}
		})
	}
}

func TestRequestIDsAreUnique(t *testing.T) {
	h := Middleware(Logger(io.Discard), nil, http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	seen := map[string]bool{}
	for range 20 {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest("GET", "/", nil))
		id := w.Header().Get("X-Request-ID")
		if seen[id] {
			t.Fatalf("duplicate request ID %q", id)
		}
		seen[id] = true
	}
}

func TestSentryDisabledMakesNoNetworkCalls(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls.Add(1) }))
	defer server.Close()
	// Even an ambient DSN must not override an explicitly disabled reporter.
	t.Setenv("SENTRY_DSN", strings.Replace(server.URL, "://", "://public@", 1)+"/1")
	client, err := NewReporter("")
	if err != nil || client != nil {
		t.Fatalf("disabled reporter: %v, %v", client, err)
	}
	Capture(client, errors.New("test error"), "test-id")
	h := Middleware(Logger(io.Discard), client, http.HandlerFunc(func(http.ResponseWriter, *http.Request) { panic("test") }))
	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/", nil))
	if got := calls.Load(); got != 0 {
		t.Fatalf("network calls = %d", got)
	}
}

func TestSentryReportsErrors(t *testing.T) {
	events := make(chan string, 4)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		events <- string(body)
		w.WriteHeader(200)
	}))
	defer server.Close()
	client, err := NewReporter(strings.Replace(server.URL, "://", "://public@", 1) + "/1")
	if err != nil {
		t.Fatal(err)
	}
	for _, status := range []int{500, 503} {
		h := Middleware(Logger(io.Discard), client, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if status == 500 {
				panic("test panic")
			}
			w.WriteHeader(status)
		}))
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest("GET", "/", nil))
		if !client.Flush(2 * time.Second) {
			t.Fatal("flush timed out")
		}
		select {
		case event := <-events:
			if !strings.Contains(event, w.Header().Get("X-Request-ID")) {
				t.Fatalf("missing request ID: %s", event)
			}
		case <-time.After(time.Second):
			t.Fatal("no event received")
		}
	}
}

// Capture the final SDK event at the transport boundary, including integrations.
type capturingTransport struct {
	events []*sentry.Event
}

func (t *capturingTransport) Configure(sentry.ClientOptions) {}
func (t *capturingTransport) SendEvent(event *sentry.Event) {
	t.events = append(t.events, event)
}
func (t *capturingTransport) Flush(time.Duration) bool              { return true }
func (t *capturingTransport) FlushWithContext(context.Context) bool { return true }
func (t *capturingTransport) Close()                                {}

func newCapturingReporter(t *testing.T) (*sentry.Client, *capturingTransport) {
	t.Helper()
	transport := &capturingTransport{}
	client, err := sentry.NewClient(sentry.ClientOptions{
		Dsn: "https://public@example.com/1", Transport: transport,
	})
	if err != nil {
		t.Fatal(err)
	}
	return client, transport
}

func TestSentryEventsExcludeSecrets(t *testing.T) {
	const secret = "postgres://u:p4ss@h/db"
	const rawError = "database connection failed: " + secret
	for _, tc := range []struct {
		name, message, typeName string
		run                     func(*testing.T, *sentry.Client) string
	}{
		{"error", "server error", "*errors.errorString", func(t *testing.T, client *sentry.Client) string {
			Capture(client, errors.New(rawError), "test-id")
			return "test-id"
		}},
		{"wrapped error", "server error", "*errors.errorString", func(t *testing.T, client *sentry.Client) string {
			Capture(client, fmt.Errorf("wrapper-secret: %w", errors.New(rawError)), "test-id")
			return "test-id"
		}},
		{"panic string", "handler panic", "string", func(t *testing.T, client *sentry.Client) string {
			h := Middleware(Logger(io.Discard), client, http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
				panic(secret)
			}))
			w := httptest.NewRecorder()
			h.ServeHTTP(w, httptest.NewRequest("GET", "/?token=request-secret", nil))
			if w.Code != http.StatusInternalServerError {
				t.Fatalf("panic status = %d", w.Code)
			}
			return w.Header().Get("X-Request-ID")
		}},
		{"panic error", "handler panic", "*errors.errorString", func(t *testing.T, client *sentry.Client) string {
			h := Middleware(Logger(io.Discard), client, http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
				panic(errors.New(rawError))
			}))
			w := httptest.NewRecorder()
			h.ServeHTTP(w, httptest.NewRequest("GET", "/", nil))
			return w.Header().Get("X-Request-ID")
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			client, transport := newCapturingReporter(t)
			id := tc.run(t, client)
			if len(transport.events) != 1 {
				t.Fatalf("events = %d", len(transport.events))
			}
			event := transport.events[0]
			body, err := json.Marshal(event)
			if err != nil {
				t.Fatal(err)
			}
			for _, sensitive := range []string{secret, "p4ss", rawError, "wrapper-secret", "request-secret"} {
				if bytes.Contains(body, []byte(sensitive)) {
					t.Fatalf("sensitive data in event: %s", body)
				}
			}
			if event.Message != tc.message || len(event.Exception) != 1 ||
				event.Exception[0].Type != tc.typeName || event.Exception[0].Value != "" {
				t.Fatalf("unexpected event: %s", body)
			}
			if id == "" || event.Tags["request_id"] != id {
				t.Fatalf("request ID missing: %s", body)
			}
		})
	}
}

func TestMiddlewareRepanicsErrAbortHandler(t *testing.T) {
	client, transport := newCapturingReporter(t)
	var log bytes.Buffer
	h := Middleware(Logger(&log), client, http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		panic(http.ErrAbortHandler)
	}))
	w := httptest.NewRecorder()
	var recovered any
	func() {
		defer func() { recovered = recover() }()
		h.ServeHTTP(w, httptest.NewRequest("GET", "/", nil))
	}()
	if recovered != http.ErrAbortHandler {
		t.Fatalf("recovered = %v, want http.ErrAbortHandler", recovered)
	}
	if w.Code != http.StatusOK || w.Body.Len() != 0 || w.Flushed {
		t.Fatalf("abort wrote response: %+v", w)
	}
	if len(transport.events) != 0 || log.Len() != 0 {
		t.Fatalf("abort reported %d events and log %q", len(transport.events), log.String())
	}
}
