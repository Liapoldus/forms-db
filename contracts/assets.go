// Package contracts exports forms-db product contracts.
package contracts

import (
	"encoding/json"

	"github.com/Liapoldus/forms-db/contracts/definition"
)

// Documents exports fresh deterministic bytes without runtime file loading.
func Documents() (map[string][]byte, error) {
	files := map[string][]byte{}
	for name, value := range definition.Documents() {
		contents, err := json.MarshalIndent(value, "", "  ")
		if err != nil {
			return nil, ErrInvalidPluginContract
		}
		files[name] = append(contents, '\n')
	}
	return files, nil
}

func Document(name string) ([]byte, error) {
	value, ok := definition.Documents()[name]
	if !ok {
		return nil, ErrInvalidPluginContract
	}
	contents, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return nil, ErrInvalidPluginContract
	}
	return append(contents, '\n'), nil
}
