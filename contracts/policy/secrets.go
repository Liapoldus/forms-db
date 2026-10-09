package policy

type SecretProfile struct {
	Version              int    `json:"version"`
	DSNField             string `json:"dsnField"`
	DSNType              string `json:"dsnType"`
	DSNDelivery          string `json:"dsnDelivery"`
	DSNGrantPurpose      string `json:"dsnGrantPurpose"`
	DSNDescription       string `json:"dsnDescription"`
	SDKGrantScope        string `json:"sdkGrantScope"`
	ActiveStorage        string `json:"activeStorage"`
	RawSecretInReload    bool   `json:"rawSecretInReload"`
	RawSecretInLogs      bool   `json:"rawSecretInLogs"`
	RawSecretInResponses bool   `json:"rawSecretInResponses"`
}

func Secrets() SecretProfile {
	return SecretProfile{
		Version:              1,
		DSNField:             "dsn",
		DSNType:              "secret",
		DSNDelivery:          "plugin-sdk-generation-scoped-grant",
		DSNGrantPurpose:      "storage-dsn",
		DSNDescription:       "Opaque Core secret reference; the plugin redeems it through Plugin SDK while applying the exact Reload generation.",
		SDKGrantScope:        "candidate-generation",
		ActiveStorage:        "in-memory-only",
		RawSecretInReload:    false,
		RawSecretInLogs:      false,
		RawSecretInResponses: false,
	}
}
