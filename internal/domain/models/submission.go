// Package models defines form submission values.
package models

type Submission struct {
	ID        string         `json:"id"`
	CreatedAt string         `json:"createdAt"`
	Site      string         `json:"site"`
	Schema    string         `json:"schemaName"`
	Data      map[string]any `json:"data"`
}
