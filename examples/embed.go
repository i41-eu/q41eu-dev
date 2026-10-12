// Package examples holds the examples' files, so Dev's page can show them.
package examples

import "embed"

// FS has README.md and the curl, python, js, go and mcp examples.
//
//go:embed README.md curl python js go mcp
var FS embed.FS
