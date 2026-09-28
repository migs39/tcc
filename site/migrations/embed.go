// Package migrations embute os arquivos SQL (formato goose) no binário do site.
package migrations

import "embed"

//go:embed *.sql
var FS embed.FS
