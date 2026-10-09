// Package record owns storage identifiers and list bounds.
package record

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
)

const (
	submissionIDPrefix                  string = "frm_"
	DefaultListRows                     int    = 50
	MaxListRows                         int    = 101
	submissionIDRandomBytes             int    = 18
	messageGenerateSubmissionIdentifier string = "generate submission identifier"
)

func NewID() (string, error) {
	bytes := make([]byte, submissionIDRandomBytes)
	if _, err := rand.Read(bytes); err != nil {
		return "", errors.New(messageGenerateSubmissionIdentifier)
	}
	return submissionIDPrefix + base64.RawURLEncoding.EncodeToString(bytes), nil
}
