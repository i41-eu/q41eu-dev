// Package examples holds the examples' files and their manifest, so Dev's site can show them.
package examples

import "embed"

// FS has README.md, examples.json (the manifest: each example, its files per language, how to
// run them), the curl, python, js, go and mcp examples, and img (the pictures they send).
//
//go:embed README.md examples.json curl python js go mcp img
var FS embed.FS
