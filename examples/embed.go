// Package examples holds the examples' files, so Dev's page can show them.
package examples

import "embed"

// FS has README.md and the curl, python and go examples.
//
//go:embed README.md curl python go
var FS embed.FS
