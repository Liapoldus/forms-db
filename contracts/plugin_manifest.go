package contracts

import (
	"encoding/json"
	"errors"
	"io/fs"
	"sync"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

var ErrInvalidPluginContract = errors.New("invalid forms-db plugin contract")

var settingsSchemaOnce sync.Once
var settingsSchema *jsonschema.Schema
var settingsSchemaError error

func PluginManifest() ([]byte, error) {
	return pluginDocument("v1/plugin.json")
}

func SettingsSchema() ([]byte, error) {
	return pluginDocument("v1/settings.schema.json")
}

func ValidateSettings(contents []byte) error {
	settingsSchemaOnce.Do(func() {
		encoded, err := SettingsSchema()
		if err != nil {
			settingsSchemaError = ErrInvalidPluginContract
			return
		}
		var source struct {
			ID string `json:"$id"`
		}
		var document any
		if json.Unmarshal(encoded, &source) != nil || source.ID == "" || json.Unmarshal(encoded, &document) != nil {
			settingsSchemaError = ErrInvalidPluginContract
			return
		}
		compiler := jsonschema.NewCompiler()
		compiler.DefaultDraft(jsonschema.Draft2020)
		if compiler.AddResource(source.ID, document) != nil {
			settingsSchemaError = ErrInvalidPluginContract
			return
		}
		settingsSchema, settingsSchemaError = compiler.Compile(source.ID)
	})
	if settingsSchemaError != nil || settingsSchema == nil {
		return ErrInvalidPluginContract
	}
	var document any
	if json.Unmarshal(contents, &document) != nil || settingsSchema.Validate(document) != nil {
		return ErrInvalidPluginContract
	}
	return nil
}

func pluginDocument(path string) ([]byte, error) {
	contents, err := fs.ReadFile(assets, path)
	if err != nil || !json.Valid(contents) {
		return nil, ErrInvalidPluginContract
	}
	return contents, nil
}
