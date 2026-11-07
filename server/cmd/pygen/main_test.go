package main

import (
	"bytes"
	"os"
	"strings"
	"testing"
)

func TestGeneration(t *testing.T) {
	spec, err := os.ReadFile("../../../api/openapi.yaml")
	if err != nil {
		t.Fatal(err)
	}
	a, err := generate(spec)
	if err != nil {
		t.Fatal(err)
	}
	b, err := generate(spec)
	if err != nil || !bytes.Equal(a, b) {
		t.Fatal("nondeterministic generation", err)
	}
	checked, err := os.ReadFile("../../../clients/python/omi_board_client/_generated.py")
	if err != nil || !bytes.Equal(a, checked) {
		t.Fatal("stale generated Python client", err)
	}
	for _, tc := range []struct{ old, new string }{
		{"type: integer", "type: number"},
		{"type: integer", "type: integer\n          format: int64"},
		{"get:", "post:"},
		{"/v1/healthz:", "/v1/{id}:"},
		{"required: [status]", "required: [unknown]"},
		{"operationId: getV1Healthz", "operationId: class"},
		{"additionalProperties: false", "additionalProperties: true"},
		{"'503':", "'400':"},
		{"$ref: '#/components/schemas/Health'", "type: string"},
	} {
		t.Run(tc.new, func(t *testing.T) {
			if _, err := generate([]byte(strings.Replace(string(spec), tc.old, tc.new, 1))); err == nil {
				t.Fatal("unsupported spec accepted")
			}
		})
	}
}
