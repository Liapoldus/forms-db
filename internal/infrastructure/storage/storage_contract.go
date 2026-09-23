package storage

import (
	"crypto/rand"
	_ "embed"
	"encoding/base64"
	"encoding/json"
	"errors"
)

//go:embed contracts/storage.json
var storageContractJSON []byte

type storageContract struct {
	SubmissionIDPrefix string `json:"submissionIdPrefix"`
	Tables             struct {
		Schemas     string `json:"schemas"`
		Submissions string `json:"submissions"`
	} `json:"tables"`
	Columns struct {
		Site       string `json:"site"`
		SchemaName string `json:"schemaName"`
		SchemaJSON string `json:"schemaJson"`
		UpdatedAt  string `json:"updatedAt"`
		ID         string `json:"id"`
		CreatedAt  string `json:"createdAt"`
		DataJSON   string `json:"dataJson"`
	} `json:"columns"`
	Indexes struct {
		SubmissionScopeCreatedAt string `json:"submissionScopeCreatedAt"`
	} `json:"indexes"`
}

func loadStorageContract() (storageContract, error) {
	var contract storageContract
	if err := json.Unmarshal(storageContractJSON, &contract); err != nil {
		return storageContract{}, errors.New("invalid forms storage contract")
	}
	if contract.SubmissionIDPrefix == "" || contract.Tables.Schemas == "" || contract.Tables.Submissions == "" ||
		contract.Columns.Site == "" || contract.Columns.SchemaName == "" || contract.Columns.SchemaJSON == "" || contract.Columns.UpdatedAt == "" ||
		contract.Columns.ID == "" || contract.Columns.CreatedAt == "" || contract.Columns.DataJSON == "" || contract.Indexes.SubmissionScopeCreatedAt == "" {
		return storageContract{}, errors.New("incomplete forms storage contract")
	}
	return contract, nil
}

func newSubmissionID() (string, error) {
	contract, err := loadStorageContract()
	if err != nil {
		return "", err
	}
	bytes := make([]byte, 18)
	if _, err := rand.Read(bytes); err != nil {
		return "", errors.New("generate submission identifier")
	}
	return contract.SubmissionIDPrefix + base64.RawURLEncoding.EncodeToString(bytes), nil
}
