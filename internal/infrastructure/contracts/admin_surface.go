package contracts

import (
	"io/fs"

	contractassets "github.com/Liapoldus/forms-db/contracts"
)

func AdminSurface() ([]byte, error) {
	return fs.ReadFile(contractassets.Files(), "v1/admin-surface.json")
}
