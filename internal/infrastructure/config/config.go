package config

import (
	"bytes"
	"encoding/json"
	"errors"
	"regexp"

	productcontracts "github.com/Liapoldus/forms-db/contracts"
	"github.com/santhosh-tekuri/jsonschema/v6"
)

type Settings struct {
	Driver          string                     `json:"driver"`
	DSNReference    string                     `json:"dsn"`
	CursorSecretRef string                     `json:"cursorSecretRef"`
	DSN             []byte                     `json:"-"`
	TablePrefix     string                     `json:"tablePrefix"`
	Schemas         map[string]json.RawMessage `json:"schemas"`
	compiledSchemas map[string]*jsonschema.Schema
	maxProperties   int
}

func (s *Settings) ApplyDSNSecret(secret []byte) error {
	contract, err := loadSecretSettingsContract()
	if err != nil || contract.DSNField == "" || contract.DSNType != "secret" || len(secret) == 0 || s.DSNReference == "" {
		return errors.New("invalid storage secret grant")
	}
	s.DSN = append(s.DSN[:0], secret...)
	return nil
}

func Apply(raw []byte) (Settings, error) {
	limits, err := productcontracts.Limits()
	if err != nil || len(raw) > limits.SettingsMaxBytes {
		return Settings{}, errors.New("invalid forms-db settings size")
	}
	if len(bytes.TrimSpace(raw)) == 0 {
		raw = []byte(`{}`)
	}
	if err := validateStrictJSONDocument(raw); err != nil {
		return Settings{}, err
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var settings Settings
	if err := decoder.Decode(&settings); err != nil {
		return Settings{}, err
	}
	if settings.Driver == "" {
		settings.Driver = "memory"
	}
	if settings.TablePrefix == "" {
		settings.TablePrefix = "form_"
	}
	if !ValidTablePrefix(settings.TablePrefix) {
		return Settings{}, errors.New("invalid table prefix")
	}
	if settings.Driver != "memory" && settings.Driver != "sqlite" && settings.Driver != "postgres" && settings.Driver != "mysql" {
		return Settings{}, errors.New("driver must be memory, sqlite, postgres, or mysql")
	}
	compiledSchemas, maxProperties, err := compileSchemas(settings.Schemas)
	if err != nil {
		return Settings{}, err
	}
	settings.compiledSchemas = compiledSchemas
	settings.maxProperties = maxProperties
	return settings, nil
}

func ValidTablePrefix(prefix string) bool {
	var contract schemaValidationContract
	if err := json.Unmarshal(schemaValidationContractJSON, &contract); err != nil || contract.TablePrefixPattern == "" {
		return false
	}
	pattern, err := regexp.Compile(contract.TablePrefixPattern)
	return err == nil && pattern.MatchString(prefix)
}
