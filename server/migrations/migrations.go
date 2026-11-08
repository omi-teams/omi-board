// Package migrations embeds the numbered SQL files applied by db.Migrate.
package migrations

import "embed"

// FS holds the NNNN_name.sql files.
//
//go:embed *.sql
var FS embed.FS
