package contracts

import (
	"encoding/json"
	"errors"
	"sync"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

var ErrInvalidServiceContract = errors.New("invalid forms-db service contract")

var settingsSchemaOnce sync.Once
var settingsSchema *jsonschema.Schema
var settingsSchemaError error

func ServiceManifest() ([]byte, error) {
	return serviceDocument("v1/service.json")
}

func SettingsSchema() ([]byte, error) {
	return serviceDocument("v1/settings.schema.json")
}

func ValidateSettings(contents []byte) error {
	limits, err := Limits()
	if err != nil || len(contents) > limits.SettingsMaxBytes {
		return ErrInvalidServiceContract
	}
	settingsSchemaOnce.Do(func() {
		encoded, err := SettingsSchema()
		if err != nil {
			settingsSchemaError = ErrInvalidServiceContract
			return
		}
		var source struct {
			ID string `json:"$id"`
		}
		var document any
		if json.Unmarshal(encoded, &source) != nil || source.ID == "" || json.Unmarshal(encoded, &document) != nil {
			settingsSchemaError = ErrInvalidServiceContract
			return
		}
		compiler := jsonschema.NewCompiler()
		compiler.DefaultDraft(jsonschema.Draft2020)
		if compiler.AddResource(source.ID, document) != nil {
			settingsSchemaError = ErrInvalidServiceContract
			return
		}
		settingsSchema, settingsSchemaError = compiler.Compile(source.ID)
	})
	if settingsSchemaError != nil || settingsSchema == nil {
		return ErrInvalidServiceContract
	}
	var document any
	if json.Unmarshal(contents, &document) != nil || settingsSchema.Validate(document) != nil {
		return ErrInvalidServiceContract
	}
	return nil
}

func serviceDocument(path string) ([]byte, error) {
	contents, err := Document(path)
	if err != nil || !json.Valid(contents) {
		return nil, ErrInvalidServiceContract
	}
	return contents, nil
}
