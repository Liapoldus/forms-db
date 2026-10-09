// Package policy owns immutable forms-db resource and security definitions.
package policy

type CursorProfile struct {
	Version                           int               `json:"version"`
	KeyBytes                          int               `json:"keyBytes"`
	GrantCapability                   string            `json:"grantCapability"`
	GrantPurpose                      string            `json:"grantPurpose"`
	GrantDomain                       string            `json:"grantDomain"`
	LifetimeSeconds                   int               `json:"lifetimeSeconds"`
	DefaultPageSize                   int               `json:"defaultPageSize"`
	MaxPageSize                       int               `json:"maxPageSize"`
	LookaheadRows                     int               `json:"lookaheadRows"`
	MaxTokenBytes                     int               `json:"maxTokenBytes"`
	TokenEncoding                     string            `json:"tokenEncoding"`
	TokenVersion                      uint8             `json:"tokenVersion"`
	NonceBytes                        int               `json:"nonceBytes"`
	AuthenticatorBytes                int               `json:"authenticatorBytes"`
	TokenLayout                       []string          `json:"tokenLayout"`
	ClaimsEncoding                    string            `json:"claimsEncoding"`
	TimestampUnit                     string            `json:"timestampUnit"`
	AuthenticatedBytes                string            `json:"authenticatedBytes"`
	AeadAdditionalData                string            `json:"aeadAdditionalData"`
	CiphertextFormat                  string            `json:"ciphertextFormat"`
	Encryption                        string            `json:"encryption"`
	EncryptionKeyLabel                string            `json:"encryptionKeyLabel"`
	Authentication                    string            `json:"authentication"`
	AuthenticationKeyLabel            string            `json:"authenticationKeyLabel"`
	KeyDerivation                     map[string]string `json:"keyDerivation"`
	ScopeDigest                       string            `json:"scopeDigest"`
	ScopeDigestEncoding               string            `json:"scopeDigestEncoding"`
	ScopeDigestInput                  string            `json:"scopeDigestInput"`
	ScopeCanonicalization             string            `json:"scopeCanonicalization"`
	ClaimsSerialization               string            `json:"claimsSerialization"`
	ScopeFields                       []string          `json:"scopeFields"`
	ScopeFilterFieldName              string            `json:"scopeFilterFieldName"`
	ScopeFilterEqualsName             string            `json:"scopeFilterEqualsName"`
	ClaimFields                       map[string]string `json:"claimFields"`
	ClaimSemantics                    map[string]string `json:"claimSemantics"`
	ExpiryRule                        string            `json:"expiryRule"`
	Ordering                          []string          `json:"ordering"`
	InvalidCursorCode                 string            `json:"invalidCursorCode"`
	UnavailableKeyCode                string            `json:"unavailableKeyCode"`
	InternalInvalidCursorError        string            `json:"internalInvalidCursorError"`
	InternalCursorKeyUnavailableError string            `json:"internalCursorKeyUnavailableError"`
}

func Cursor() CursorProfile {
	return CursorProfile{ // #nosec G101 -- Public KDF labels and algorithm names; no keys or credentials are embedded.
		Version:                           1,
		KeyBytes:                          32,
		GrantCapability:                   "forms.list",
		GrantPurpose:                      "cursor-signing",
		GrantDomain:                       "forms-db.cursor",
		LifetimeSeconds:                   900,
		DefaultPageSize:                   50,
		MaxPageSize:                       100,
		LookaheadRows:                     1,
		MaxTokenBytes:                     4096,
		TokenEncoding:                     "base64url-no-padding",
		TokenVersion:                      1,
		NonceBytes:                        12,
		AuthenticatorBytes:                32,
		TokenLayout:                       []string{"version", "nonce", "ciphertext", "authenticator"},
		ClaimsEncoding:                    "объект JSON в UTF-8",
		TimestampUnit:                     "наносекунды Unix time",
		AuthenticatedBytes:                "version || nonce || ciphertext",
		AeadAdditionalData:                "байт version",
		CiphertextFormat:                  "шифротекст AES-GCM, затем добавленный GCM tag",
		Encryption:                        "AES-256-GCM",
		EncryptionKeyLabel:                "liapoldus.forms-db.cursor.v1.encryption",
		Authentication:                    "HMAC-SHA-256",
		AuthenticationKeyLabel:            "liapoldus.forms-db.cursor.v1.authentication",
		KeyDerivation:                     map[string]string{"encryption": "HMAC-SHA-256 исходного ключа с encryptionKeyLabel", "authentication": "HMAC-SHA-256 исходного ключа с authenticationKeyLabel"},
		ScopeDigest:                       "SHA-256-canonical-json",
		ScopeDigestEncoding:               "base64url без padding",
		ScopeDigestInput:                  "канонический JSON-объект site, schemaName и filter; отсутствующий filter равен null; поля filter: field и equals",
		ScopeCanonicalization:             "filter.equals разбирается с JSON UseNumber; Go encoding/json сериализует scope компактно и сортирует ключи",
		ClaimsSerialization:               "Go encoding/json сериализует claims компактно в UTF-8 и сортирует ключи объекта лексикографически",
		ScopeFields:                       []string{"site", "schemaName", "filter"},
		ScopeFilterFieldName:              "field",
		ScopeFilterEqualsName:             "equals",
		ClaimFields:                       map[string]string{"scopeDigest": "scopeDigest", "createdAt": "createdAt", "id": "id", "issuedAt": "issuedAt", "expiresAt": "expiresAt"},
		ClaimSemantics:                    map[string]string{"scopeDigest": "SHA-256 digest канонического scope, закодированный в base64url без padding", "createdAt": "createdAt последней записи предыдущей страницы", "id": "id последней записи предыдущей страницы", "issuedAt": "время выпуска cursor в единицах timestampUnit", "expiresAt": "issuedAt плюс lifetimeSeconds, переведённые в timestampUnit"},
		ExpiryRule:                        "issuedAt <= now < expiresAt; expiresAt - issuedAt равно lifetimeSeconds после перевода в timestampUnit",
		Ordering:                          []string{"createdAt DESC", "id DESC"},
		InvalidCursorCode:                 "validation_failed",
		UnavailableKeyCode:                "storage_unavailable",
		InternalInvalidCursorError:        "invalid cursor",
		InternalCursorKeyUnavailableError: "cursor signing key unavailable",
	}
}
