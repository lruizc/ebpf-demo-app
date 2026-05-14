// Package web provides the embedded filesystem for HTML templates and static assets.
package web

import "embed"

//go:embed templates static
var FS embed.FS
