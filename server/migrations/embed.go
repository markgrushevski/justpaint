// Package migrations carries the goose migration files as an embedded
// filesystem, so the server can apply them itself at boot.
//
// The .sql files stay exactly where the goose CLI expects them
// (`goose -dir server/migrations …` still works, unchanged): this file only
// makes the same directory readable from inside the binary, for hosts where
// running a one-off command is not an option.
package migrations

import "embed"

// FS holds every migration, in filename order — the order goose applies them in.
//
//go:embed *.sql
var FS embed.FS
