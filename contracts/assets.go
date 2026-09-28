package contracts

import (
	"embed"
	"io/fs"
)

//go:embed v1
var assets embed.FS

func Files() fs.FS {
	return assets
}
