// Package web embeds the built browser UI (see web/README.md for the frontend
// source) so it can be served directly from the tlw binary without any
// external files.
package web

import "embed"

// DistFS holds the production build output (web/dist), produced by
// `task web:build`. Run that after changing anything under web/src.
//
//go:embed all:dist
var DistFS embed.FS
