// Package roadmapseed embeds learning path JSON seed files into the binary.
package roadmapseed

import "embed"

// FS embeds learning path JSON seed files.
//
//go:embed *.json
var FS embed.FS

// Dir specifies the root directory of the embedded files.
const Dir = "."

