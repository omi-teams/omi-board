// Package api implements the generated v1 HTTP contract.
package api

//go:generate go tool oapi-codegen -config oapi-codegen.yaml ../../../api/openapi.yaml
//go:generate go run ../../cmd/pygen -spec ../../../api/openapi.yaml -out ../../../clients/python/omi_board_client/_generated.py
