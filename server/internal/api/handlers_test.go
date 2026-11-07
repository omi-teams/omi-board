package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/getkin/kin-openapi/openapi3filter"
	"github.com/getkin/kin-openapi/routers/legacy"
)

type fakeDB struct{ err error }

func (f fakeDB) Ping(context.Context) error { return f.err }

func TestContractAndHandler(t *testing.T) {
	doc, err := openapi3.NewLoader().LoadFromFile("../../../api/openapi.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if err = doc.Validate(context.Background()); err != nil {
		t.Fatal(err)
	}
	problem := doc.Components.Schemas["Problem"].Value
	if !strings.Contains(","+strings.Join(problem.Required, ",")+",", ",code,") {
		t.Fatal("Problem.code must be required")
	}
	router, err := legacy.NewRouter(doc)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name   string
		err    error
		status int
		media  string
	}{
		{"healthy", nil, 200, "application/json"},
		{"unavailable", errors.New("private database credentials"), 503, "application/problem+json"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/v1/healthz", nil)
			rec := httptest.NewRecorder()
			Handler(NewStrictHandler(Server{DB: fakeDB{tc.err}}, nil)).ServeHTTP(rec, req)
			if rec.Code != tc.status || rec.Header().Get("Content-Type") != tc.media {
				t.Fatalf("response: %d %v", rec.Code, rec.Header())
			}
			route, params, err := router.FindRoute(req)
			if err != nil {
				t.Fatal(err)
			}
			input := &openapi3filter.ResponseValidationInput{RequestValidationInput: &openapi3filter.RequestValidationInput{Request: req, PathParams: params, Route: route}, Status: rec.Code, Header: rec.Header()}
			input.SetBodyBytes(rec.Body.Bytes())
			if err := openapi3filter.ValidateResponse(context.Background(), input); err != nil {
				t.Fatal(err)
			}
			if tc.status == 503 {
				var p Problem
				if err := json.Unmarshal(rec.Body.Bytes(), &p); err != nil {
					t.Fatal(err)
				}
				if p.Code != "database_unavailable" || strings.Contains(rec.Body.String(), "credentials") {
					t.Fatal(rec.Body.String())
				}
			}
		})
	}
}

func TestEventSchemaDocumentation(t *testing.T) {
	data, err := os.ReadFile("../../../api/events/envelope.schema.json")
	if err != nil {
		t.Fatal(err)
	}
	var s struct {
		Schema     string         `json:"$schema"`
		Required   []string       `json:"required"`
		Properties map[string]any `json:"properties"`
	}
	if err = json.Unmarshal(data, &s); err != nil {
		t.Fatal(err)
	}
	if s.Schema != "https://json-schema.org/draft/2020-12/schema" || strings.Join(s.Required, ",") != "seq,type,data" {
		t.Fatal("invalid envelope contract")
	}
	for _, field := range []string{"seq", "type", "data", "project_id", "ticket_id"} {
		if s.Properties[field] == nil {
			t.Fatalf("missing %s", field)
		}
	}
	readme, err := os.ReadFile("../../../api/events/README.md")
	if err != nil || !strings.Contains(string(readme), "JSON Schema") {
		t.Fatal("missing JSON Schema documentation", err)
	}
}
