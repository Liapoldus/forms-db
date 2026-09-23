package contracts

import (
	"io/fs"

	"github.com/Liapoldus/pluginprotocol"
)

func AdminSurface() ([]byte, error) {
	return fs.ReadFile(pluginprotocol.ContractFiles(), "contracts/forms-db/v1/admin-surface.json")
}
