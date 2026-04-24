//go:build !dev

package webui

import (
	"embed"
	"io/fs"
)

// Dist contains the pre-rendered Nuxt admin application.
//
//go:embed all:dist
var embeddedDist embed.FS

var Dist fs.FS = embeddedDist
