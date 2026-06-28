package web

import "embed"

// Files holds our embedded templates and static files.
//
//go:embed templates static
var Files embed.FS
