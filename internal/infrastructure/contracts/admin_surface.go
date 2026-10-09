// Package contracts adapts the product-owned admin surface.
package contracts

import contractassets "github.com/Liapoldus/forms-db/contracts"

func AdminSurface() ([]byte, error) {
	return contractassets.Document("v1/admin-surface.json")
}
