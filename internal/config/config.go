package config

import (
	"bytes"
	"encoding/json"
	"errors"
)

type Settings struct {
	Driver      string                     `json:"driver"`
	DSN         string                     `json:"dsn"`
	TablePrefix string                     `json:"tablePrefix"`
	Schemas     map[string]json.RawMessage `json:"schemas"`
}

func Apply(raw []byte) (Settings, error) {
	if len(bytes.TrimSpace(raw)) == 0 {
		raw = []byte(`{}`)
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
	if settings.Driver != "memory" && settings.Driver != "sqlite" && settings.Driver != "postgres" && settings.Driver != "mysql" {
		return Settings{}, errors.New("driver must be memory, sqlite, postgres, or mysql")
	}
	return settings, nil
}
