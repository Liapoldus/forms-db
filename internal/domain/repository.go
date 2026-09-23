package domain

import "context"

type Submission struct {
	ID        string         `json:"id"`
	CreatedAt string         `json:"createdAt"`
	Site      string         `json:"site"`
	Schema    string         `json:"schemaName"`
	Data      map[string]any `json:"data"`
}

// Repository is the seam for the future SQLite/PostgreSQL/MySQL adapters.
type Repository interface {
	Submit(context.Context, Submission) (Submission, error)
	List(context.Context, string, string, int) ([]Submission, error)
	Delete(context.Context, string) error
}
