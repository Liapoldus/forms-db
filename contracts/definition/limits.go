package definition

import (
	"encoding/json"
	"github.com/Liapoldus/forms-db/contracts/policy"
)

func RuntimeLimits() any {
	// ResourceLimits contains JSON primitives only; marshaling cannot fail.
	encoded, err := json.Marshal(policy.Limits())
	if err != nil {
		panic(err)
	}
	var document map[string]any
	if err := json.Unmarshal(encoded, &document); err != nil {
		panic(err)
	}
	document["$id"] = "https://github.com/Liapoldus/forms-db/contracts/v1/runtime-limits.json"
	document["limitsSources"] = map[string]any{
		"pageSizeMaximum":    []string{"contracts/v1/list-request.schema.json", "contracts/v1/cursor.json"},
		"cursorMaximumBytes": []string{"contracts/v1/list-request.schema.json", "contracts/v1/cursor.json"},
	}
	return document
}
