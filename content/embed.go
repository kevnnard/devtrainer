// Package content embeds the study content: one YAML file per track under
// <track>/<track>.yaml. The schema is documented in SCHEMA.md; load it with
// internal/content.Load(content.FS).
package content

import "embed"

// FS holds every track file.
//
//go:embed */*.yaml
var FS embed.FS
