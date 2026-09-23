package models

import "encoding/json"

type SubmissionFilter struct {
	Field  string
	Equals json.RawMessage
}
