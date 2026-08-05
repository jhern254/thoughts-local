package migrations

import "embed"

// Files contains the SQL migrations shipped with the application.
//
//go:embed *.up.sql
var Files embed.FS
