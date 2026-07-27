// Package templates holds the default templates compiled into the binary.
// Users override them by name from a project or user template directory
package templates

import "embed"

// Root is the directory the templates are embedded under
const Root string = "files"

//go:embed all:files
var FS embed.FS
