// Package migration embeds SQL schema migration files into the binary for goose.
package migration

import "embed"

// FS embeds all Postgres SQL migration files.
//
//go:embed *.sql
var FS embed.FS

