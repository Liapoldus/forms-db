package config

import (
	_ "embed"
	"encoding/json"
	"errors"
	"net/url"
	"regexp"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

//go:embed contracts/schema-validation.json
var schemaValidationContractJSON []byte

type schemaValidationContract struct {
	SchemaNamePattern       string `json:"schemaNamePattern"`
	TablePrefixPattern      string `json:"tablePrefixPattern"`
	Draft2020Schema         string `json:"draft2020SchemaUrl"`
	ResourceBase            string `json:"resourceBase"`
	MaxSubmissionProperties int    `json:"maxSubmissionProperties"`
}

type unavailableSchemaLoader struct{}

func (unavailableSchemaLoader) Load(string) (any, error) {
	return nil, errors.New("external schema resource is unavailable")
}

func compileSchemas(schemas map[string]json.RawMessage) (map[string]*jsonschema.Schema, int, error) {
	var contract schemaValidationContract
	if err := json.Unmarshal(schemaValidationContractJSON, &contract); err != nil {
		return nil, 0, errors.New("invalid schema validation contract")
	}
	namePattern, err := regexp.Compile(contract.SchemaNamePattern)
	if err != nil || contract.SchemaNamePattern == "" || contract.Draft2020Schema == "" || contract.ResourceBase == "" || contract.MaxSubmissionProperties < 1 {
		return nil, 0, errors.New("invalid schema validation contract")
	}
	compiled := make(map[string]*jsonschema.Schema, len(schemas))
	if len(schemas) == 0 {
		return compiled, contract.MaxSubmissionProperties, nil
	}
	compiler := jsonschema.NewCompiler()
	compiler.DefaultDraft(jsonschema.Draft2020)
	compiler.AssertVocabs()
	compiler.UseLoader(unavailableSchemaLoader{})
	resources := make(map[string]string, len(schemas))
	for name, raw := range schemas {
		if !namePattern.MatchString(name) {
			return nil, 0, errors.New("invalid registered schema name")
		}
		var document any
		if err := json.Unmarshal(raw, &document); err != nil {
			return nil, 0, errors.New("invalid registered JSON Schema")
		}
		if schema, ok := document.(map[string]any); ok {
			if draft, exists := schema["$schema"]; exists && draft != contract.Draft2020Schema {
				return nil, 0, errors.New("registered schema must use JSON Schema Draft 2020-12")
			}
		}
		resource := contract.ResourceBase + url.PathEscape(name)
		if err := compiler.AddResource(resource, document); err != nil {
			return nil, 0, errors.New("invalid registered JSON Schema")
		}
		resources[name] = resource
	}
	for name, resource := range resources {
		schema, err := compiler.Compile(resource)
		if err != nil {
			return nil, 0, errors.New("invalid registered JSON Schema")
		}
		compiled[name] = schema
	}
	return compiled, contract.MaxSubmissionProperties, nil
}

func (s Settings) ValidateSubmissionData(schemaName string, data any) error {
	schema, exists := s.compiledSchemas[schemaName]
	if !exists {
		return errors.New("submission schema is not registered")
	}
	object, ok := data.(map[string]any)
	if !ok || len(object) > s.maxProperties {
		return errors.New("submission data exceeds the contract limit")
	}
	return schema.Validate(data)
}

func (s Settings) HasSchema(schemaName string) bool {
	_, exists := s.compiledSchemas[schemaName]
	return exists
}

func (s Settings) AllowsFilterField(schemaName, field string) bool {
	schema, exists := s.compiledSchemas[schemaName]
	if !exists {
		return false
	}
	return schemaAllowsField(schema, field, make(map[*jsonschema.Schema]struct{}))
}

func schemaAllowsField(schema *jsonschema.Schema, field string, visited map[*jsonschema.Schema]struct{}) bool {
	if schema == nil {
		return false
	}
	if schema.Bool != nil {
		return *schema.Bool
	}
	if _, exists := visited[schema]; exists {
		return true
	}
	visited[schema] = struct{}{}
	allowed := false
	if _, exists := schema.Properties[field]; exists {
		allowed = true
	}
	for pattern := range schema.PatternProperties {
		if pattern.MatchString(field) {
			allowed = true
			break
		}
	}
	if !allowed {
		allowed = additionalPropertiesAllowed(schema.AdditionalProperties)
		if schema.AdditionalProperties == nil && schema.UnevaluatedProperties != nil && schema.UnevaluatedProperties.Bool != nil && !*schema.UnevaluatedProperties.Bool {
			allowed = false
		}
	}
	if !allowed {
		return false
	}
	for _, reference := range []*jsonschema.Schema{schema.Ref, schema.RecursiveRef} {
		if reference != nil && !schemaAllowsField(reference, field, visited) {
			return false
		}
	}
	for _, candidate := range schema.AllOf {
		if !schemaAllowsField(candidate, field, visited) {
			return false
		}
	}
	for _, candidates := range [][]*jsonschema.Schema{schema.AnyOf, schema.OneOf} {
		if len(candidates) == 0 {
			continue
		}
		matched := false
		for _, candidate := range candidates {
			if schemaAllowsField(candidate, field, visited) {
				matched = true
				break
			}
		}
		if !matched {
			return false
		}
	}
	if schema.If != nil && (schema.Then != nil || schema.Else != nil) {
		thenAllows := schema.Then != nil && schemaAllowsField(schema.Then, field, visited)
		elseAllows := schema.Else != nil && schemaAllowsField(schema.Else, field, visited)
		if !thenAllows && !elseAllows {
			return false
		}
	}
	return true
}

func additionalPropertiesAllowed(value any) bool {
	switch value := value.(type) {
	case nil, *jsonschema.Schema:
		return true
	case bool:
		return value
	default:
		return false
	}
}
