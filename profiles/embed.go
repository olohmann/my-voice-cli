package profiles

import "embed"

//go:embed *.md
var DefaultProfiles embed.FS
