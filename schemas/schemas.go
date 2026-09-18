// Package schemas embeds the JSON Schemas that describe kube2e file formats.
package schemas

import _ "embed"

// Case holds the JSON Schema for a kube2e case file.
//
//go:embed case.schema.json
var Case []byte
