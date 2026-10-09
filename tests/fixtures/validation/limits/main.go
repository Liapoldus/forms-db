package main

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/Liapoldus/forms-db/tests/fixtures/support"
	"strings"

	"github.com/Liapoldus/forms-db/contracts"
	"github.com/Liapoldus/forms-db/internal/application"
	"github.com/Liapoldus/forms-db/internal/domain/interfaces"
	"github.com/Liapoldus/forms-db/internal/infrastructure/config"
	"github.com/Liapoldus/forms-db/internal/infrastructure/storage/memory"
	"github.com/Liapoldus/forms-db/internal/presentation/peerplugin"
	"github.com/Liapoldus/forms-db/internal/presentation/restplugin"
	sdkmodels "github.com/Liapoldus/plugin-sdk/domain/models"
	"github.com/Liapoldus/pluginprotocol/v2/presentation/peer"
)

func main() {
	limits, err := contracts.Limits()
	if err != nil {
		panic(err)
	}
	baseSettings := []byte(`{"driver":"memory","schemas":{"contact":{"type":"object","properties":{"email":{"type":"string"}}}}}`)
	settingsAtLimit := append(append([]byte(nil), baseSettings...), []byte(strings.Repeat(" ", limits.SettingsMaxBytes-len(baseSettings)))...)
	settingsOverLimit := append(settingsAtLimit, ' ')
	_, atLimitErr := config.Apply(settingsAtLimit)
	_, overLimitErr := config.Apply(settingsOverLimit)
	manifestAtLimitErr := contracts.ValidateSettings(settingsAtLimit)
	manifestOverLimitErr := contracts.ValidateSettings(settingsOverLimit)

	depthAtLimit := schemaWithDepth(limits.SchemaMaxDepth)
	depthOverLimit := schemaWithDepth(limits.SchemaMaxDepth + 1)
	_, depthAcceptedErr := config.Apply(encodeSettings(depthAtLimit))
	_, depthRejectedErr := config.Apply(encodeSettings(depthOverLimit))

	fieldsAtLimit := schemaWithFields(limits.FieldsPerFormMax)
	fieldsOverLimit := schemaWithFields(limits.FieldsPerFormMax + 1)
	_, fieldsAcceptedErr := config.Apply(encodeSettings(fieldsAtLimit))
	_, fieldsRejectedErr := config.Apply(encodeSettings(fieldsOverLimit))

	submissionAtLimitAccepted, submissionOverLimitRejected := testSubmissionLimit(limits.SubmissionRequestMaxBytes)
	output, fixtureErr := json.Marshal(map[string]bool{
		"settingsBytesAtLimitAccepted":      atLimitErr == nil,
		"settingsBytesOverLimitRejected":    overLimitErr != nil,
		"manifestSettingsAtLimitAccepted":   manifestAtLimitErr == nil,
		"manifestSettingsOverLimitRejected": manifestOverLimitErr != nil,
		"schemaDepthAtLimitAccepted":        depthAcceptedErr == nil,
		"schemaDepthOverLimitRejected":      depthRejectedErr != nil,
		"schemaFieldsAtLimitAccepted":       fieldsAcceptedErr == nil,
		"schemaFieldsOverLimitRejected":     fieldsRejectedErr != nil,
		"submissionAtLimitAccepted":         submissionAtLimitAccepted,
		"submissionOverLimitRejected":       submissionOverLimitRejected,
	})
	support.Check(fixtureErr)
	support.Written(fmt.Println(string(output)))
}

func encodeSettings(schema map[string]any) []byte {
	encoded, err := json.Marshal(map[string]any{"driver": "memory", "schemas": map[string]any{"contact": schema}})
	if err != nil {
		panic(err)
	}
	return encoded
}

func schemaWithDepth(depth int) map[string]any {
	current := map[string]any{"type": "string"}
	for level := 1; level < depth; level++ {
		current = map[string]any{"allOf": []any{current}}
	}
	return current
}

func schemaWithFields(count int) map[string]any {
	properties := make(map[string]any, count)
	for index := 0; index < count; index++ {
		properties[fmt.Sprintf("field%d", index)] = map[string]any{"type": "string"}
	}
	return map[string]any{"type": "object", "properties": properties}
}

func testSubmissionLimit(maxBytes int) (bool, bool) {
	repository := memory.New()
	active, err := restplugin.New(application.Service{Repository: memory.New()}, func(context.Context, config.Settings) (interfaces.Repository, error) {
		return repository, nil
	}, nil)
	if err != nil {
		panic(err)
	}
	settings := []byte(`{"driver":"memory","schemas":{"contact":{"type":"object","properties":{"email":{"type":"string"}},"required":["email"]}}}`)
	configuration, err := sdkmodels.NewConfiguration("generation-1", "1", sdkmodels.Digest(settings), settings)
	if err != nil || active.Apply(context.Background(), configuration) != nil {
		panic("could not apply valid settings")
	}
	handler, err := peerplugin.New(active, nil, peer.AllowAll{})
	if err != nil {
		panic(err)
	}
	invoke := func(payload []byte) int {
		call := peer.Call{Method: "forms.submit", Payload: payload}
		serve, err := handler.PrepareCall(context.Background(), peer.PeerIdentity{URI: "spiffe://example/test"}, call)
		if err != nil {
			panic(err)
		}
		result, err := serve(context.Background(), call)
		if err != nil {
			panic(err)
		}
		var response struct {
			Status int `json:"status"`
		}
		if json.Unmarshal(result.Payload, &response) != nil {
			panic("invalid submit response")
		}
		return response.Status
	}
	makePayload := func(length int) []byte {
		base, fixtureErr := json.Marshal(map[string]any{"site": "site", "schemaName": "contact", "data": map[string]string{"email": ""}})
		support.Check(fixtureErr)
		return []byte(strings.Replace(string(base), `"email":""`, `"email":"`+strings.Repeat("a", length)+`"`, 1))
	}
	minimumPayload := makePayload(0)
	atLimitPayload := makePayload(maxBytes - len(minimumPayload))
	if len(atLimitPayload) != maxBytes {
		panic("could not build exact-boundary submit payload")
	}
	return invoke(atLimitPayload) == 200, invoke(append(atLimitPayload, ' ')) == 422
}
