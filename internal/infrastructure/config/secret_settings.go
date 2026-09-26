package config

import (
	_ "embed"
	"encoding/json"
	"errors"
)

//go:embed contracts/secret-settings.json
var secretSettingsContractJSON []byte

type secretSettingsContract struct {
	Version                int    `json:"version"`
	DSNField               string `json:"dsnField"`
	DSNType                string `json:"dsnType"`
	DSNDelivery            string `json:"dsnDelivery"`
	DSNGrantPurpose        string `json:"dsnGrantPurpose"`
	DSNDescription         string `json:"dsnDescription"`
	ConfigGrantScope       string `json:"configGrantScope"`
	ActiveStorage          string `json:"activeStorage"`
	RawSecretInConfigApply bool   `json:"rawSecretInConfigApply"`
	RawSecretInLogs        bool   `json:"rawSecretInLogs"`
	RawSecretInResponses   bool   `json:"rawSecretInResponses"`
}

func loadSecretSettingsContract() (secretSettingsContract, error) {
	var contract secretSettingsContract
	if err := json.Unmarshal(secretSettingsContractJSON, &contract); err != nil || contract.Version != 1 ||
		contract.DSNField == "" || contract.DSNType != "secret" || contract.DSNDelivery == "" || contract.DSNGrantPurpose == "" || contract.DSNDescription == "" ||
		contract.ConfigGrantScope == "" || contract.ActiveStorage == "" || contract.RawSecretInConfigApply ||
		contract.RawSecretInLogs || contract.RawSecretInResponses {
		return secretSettingsContract{}, errors.New("invalid secret settings contract")
	}
	return contract, nil
}

func DSNSecretDescription() (string, bool) {
	contract, err := loadSecretSettingsContract()
	if err != nil {
		return "", false
	}
	return contract.DSNDescription, true
}

func DSNSecretGrantPurpose() (string, bool) {
	contract, err := loadSecretSettingsContract()
	if err != nil {
		return "", false
	}
	return contract.DSNGrantPurpose, true
}
