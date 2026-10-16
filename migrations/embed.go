// Package migrations embeds the versioned SQL migration files so the
// binary can apply them without needing the source tree at runtime.
package migrations

import "embed"

//go:embed *.sql
var FS embed.FS
