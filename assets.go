package app

import "embed"

//go:embed web/dist
var DistFS embed.FS
