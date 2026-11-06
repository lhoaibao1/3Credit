// Package web embeds templates and static assets.
package web

import "embed"

//go:embed templates/*.html static policies/*.md
var FS embed.FS
