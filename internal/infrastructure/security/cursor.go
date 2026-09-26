package security

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	_ "embed"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"time"

	"github.com/Liapoldus/forms-db/internal/domain/models"
)

//go:embed contracts/cursor.json
var cursorContractJSON []byte

var (
	ErrInvalidCursor        error
	ErrCursorKeyUnavailable error
)

func init() {
	contract, err := loadCursorContract()
	if err != nil {
		return
	}
	ErrInvalidCursor = errors.New(contract.InternalInvalidCursor)
	ErrCursorKeyUnavailable = errors.New(contract.InternalKeyUnavailable)
}

type CursorScope struct {
	Site       string
	SchemaName string
	Filter     *models.SubmissionFilter
}

type CursorSigner struct {
	contract cursorContract
	macKey   []byte
	block    cipher.AEAD
}

type cursorContract struct {
	Version                int               `json:"version"`
	KeyBytes               int               `json:"keyBytes"`
	GrantCapability        string            `json:"grantCapability"`
	GrantPurpose           string            `json:"grantPurpose"`
	GrantDomain            string            `json:"grantDomain"`
	LifetimeSeconds        int64             `json:"lifetimeSeconds"`
	DefaultPageSize        int               `json:"defaultPageSize"`
	MaxPageSize            int               `json:"maxPageSize"`
	LookaheadRows          int               `json:"lookaheadRows"`
	MaxTokenBytes          int               `json:"maxTokenBytes"`
	TokenVersion           int               `json:"tokenVersion"`
	NonceBytes             int               `json:"nonceBytes"`
	AuthenticatorBytes     int               `json:"authenticatorBytes"`
	Encryption             string            `json:"encryption"`
	EncryptionKeyLabel     string            `json:"encryptionKeyLabel"`
	Authentication         string            `json:"authentication"`
	AuthenticationKeyLabel string            `json:"authenticationKeyLabel"`
	ScopeFields            []string          `json:"scopeFields"`
	ScopeFilterFieldName   string            `json:"scopeFilterFieldName"`
	ScopeFilterEqualsName  string            `json:"scopeFilterEqualsName"`
	ClaimFields            map[string]string `json:"claimFields"`
	InvalidCursorCode      string            `json:"invalidCursorCode"`
	UnavailableKeyCode     string            `json:"unavailableKeyCode"`
	InternalInvalidCursor  string            `json:"internalInvalidCursorError"`
	InternalKeyUnavailable string            `json:"internalCursorKeyUnavailableError"`
}

func CursorErrorCodes() (invalidCursor, unavailableKey string) {
	contract, err := loadCursorContract()
	if err != nil {
		return "", ""
	}
	return contract.InvalidCursorCode, contract.UnavailableKeyCode
}

func CursorPageLimits() (defaultPageSize, maxPageSize, lookaheadRows int, ok bool) {
	contract, err := loadCursorContract()
	if err != nil {
		return 0, 0, 0, false
	}
	return contract.DefaultPageSize, contract.MaxPageSize, contract.LookaheadRows, true
}

// CursorGrantScope returns the request-scoped grant selectors from the
// versioned security contract. It contains no secret material.
func CursorGrantScope() (capability, purpose, domain string, ok bool) {
	contract, err := loadCursorContract()
	if err != nil {
		return "", "", "", false
	}
	return contract.GrantCapability, contract.GrantPurpose, contract.GrantDomain, true
}

func NewCursorSigner(key []byte) (*CursorSigner, error) {
	contract, err := loadCursorContract()
	if err != nil {
		return nil, ErrCursorKeyUnavailable
	}
	return newCursorSigner(contract, key)
}

func (s *CursorSigner) Close() {
	if s == nil {
		return
	}
	clear(s.macKey)
	s.macKey = nil
	s.block = nil
}

func newCursorSigner(contract cursorContract, key []byte) (*CursorSigner, error) {
	if len(key) != contract.KeyBytes || contract.LifetimeSeconds < 1 || contract.TokenVersion < 0 || contract.TokenVersion > 255 ||
		contract.NonceBytes < 1 || contract.AuthenticatorBytes != sha256.Size || contract.Encryption == "" ||
		contract.Authentication == "" || contract.EncryptionKeyLabel == "" || contract.AuthenticationKeyLabel == "" {
		return nil, ErrCursorKeyUnavailable
	}
	encKey := deriveKey(key, contract.EncryptionKeyLabel)
	macKey := deriveKey(key, contract.AuthenticationKeyLabel)
	block, err := aes.NewCipher(encKey)
	clear(encKey)
	if err != nil {
		clear(macKey)
		return nil, ErrCursorKeyUnavailable
	}
	aead, err := cipher.NewGCM(block)
	if err != nil || aead.NonceSize() != contract.NonceBytes {
		clear(macKey)
		return nil, ErrCursorKeyUnavailable
	}
	return &CursorSigner{contract: contract, macKey: macKey, block: aead}, nil
}

func deriveKey(secret []byte, label string) []byte {
	mac := hmac.New(sha256.New, secret)
	_, _ = io.WriteString(mac, label)
	return mac.Sum(nil)
}

func (s *CursorSigner) Encode(scope CursorScope, position models.SubmissionCursor, issuedAt time.Time) (string, error) {
	if s == nil || s.block == nil || len(s.macKey) == 0 || position.CreatedAt == "" || position.ID == "" {
		return "", ErrCursorKeyUnavailable
	}
	scopeDigest, err := s.scopeDigest(scope)
	if err != nil {
		return "", ErrInvalidCursor
	}
	claims := map[string]any{
		s.contract.ClaimFields["scopeDigest"]: base64.RawURLEncoding.EncodeToString(scopeDigest),
		s.contract.ClaimFields["createdAt"]:   position.CreatedAt,
		s.contract.ClaimFields["id"]:          position.ID,
		s.contract.ClaimFields["issuedAt"]:    issuedAt.UnixNano(),
		s.contract.ClaimFields["expiresAt"]:   issuedAt.Add(time.Duration(s.contract.LifetimeSeconds) * time.Second).UnixNano(),
	}
	plain, err := json.Marshal(claims)
	if err != nil {
		return "", ErrInvalidCursor
	}
	nonce := make([]byte, s.block.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", ErrCursorKeyUnavailable
	}
	version := byte(s.contract.TokenVersion)
	ciphertext := s.block.Seal(nil, nonce, plain, []byte{version})
	unsigned := make([]byte, 1+len(nonce)+len(ciphertext))
	unsigned[0] = version
	copy(unsigned[1:], nonce)
	copy(unsigned[1+len(nonce):], ciphertext)
	mac := hmac.New(sha256.New, s.macKey)
	_, _ = mac.Write(unsigned)
	token := append(unsigned, mac.Sum(nil)...)
	return base64.RawURLEncoding.EncodeToString(token), nil
}

func (s *CursorSigner) Decode(token string, scope CursorScope, now time.Time) (models.SubmissionCursor, error) {
	if s == nil || s.block == nil || len(s.macKey) == 0 || token == "" || len(token) > s.contract.MaxTokenBytes {
		return models.SubmissionCursor{}, ErrInvalidCursor
	}
	encoded, err := base64.RawURLEncoding.DecodeString(token)
	minimum := 1 + s.block.NonceSize() + s.block.Overhead() + s.contract.AuthenticatorBytes
	if err != nil || len(encoded) < minimum || int(encoded[0]) != s.contract.TokenVersion {
		return models.SubmissionCursor{}, ErrInvalidCursor
	}
	macStart := len(encoded) - s.contract.AuthenticatorBytes
	mac := hmac.New(sha256.New, s.macKey)
	_, _ = mac.Write(encoded[:macStart])
	if !hmac.Equal(encoded[macStart:], mac.Sum(nil)) {
		return models.SubmissionCursor{}, ErrInvalidCursor
	}
	nonceStart := 1
	nonceEnd := nonceStart + s.block.NonceSize()
	plain, err := s.block.Open(nil, encoded[nonceStart:nonceEnd], encoded[nonceEnd:macStart], encoded[:1])
	if err != nil {
		return models.SubmissionCursor{}, ErrInvalidCursor
	}
	claims := make(map[string]json.RawMessage)
	if err := json.Unmarshal(plain, &claims); err != nil {
		return models.SubmissionCursor{}, ErrInvalidCursor
	}
	var digestText, createdAt, id string
	var issuedAt, expiresAt int64
	if json.Unmarshal(claims[s.contract.ClaimFields["scopeDigest"]], &digestText) != nil ||
		json.Unmarshal(claims[s.contract.ClaimFields["createdAt"]], &createdAt) != nil ||
		json.Unmarshal(claims[s.contract.ClaimFields["id"]], &id) != nil ||
		json.Unmarshal(claims[s.contract.ClaimFields["issuedAt"]], &issuedAt) != nil ||
		json.Unmarshal(claims[s.contract.ClaimFields["expiresAt"]], &expiresAt) != nil || createdAt == "" || id == "" {
		return models.SubmissionCursor{}, ErrInvalidCursor
	}
	digest, err := base64.RawURLEncoding.DecodeString(digestText)
	if err != nil || len(digest) != sha256.Size {
		return models.SubmissionCursor{}, ErrInvalidCursor
	}
	wantedDigest, err := s.scopeDigest(scope)
	if err != nil || subtle.ConstantTimeCompare(digest, wantedDigest) != 1 {
		return models.SubmissionCursor{}, ErrInvalidCursor
	}
	nowNanos := now.UnixNano()
	if issuedAt > nowNanos || expiresAt <= nowNanos || expiresAt-issuedAt != int64(time.Duration(s.contract.LifetimeSeconds)*time.Second) {
		return models.SubmissionCursor{}, ErrInvalidCursor
	}
	return models.SubmissionCursor{CreatedAt: createdAt, ID: id}, nil
}

func (s *CursorSigner) scopeDigest(scope CursorScope) ([]byte, error) {
	if scope.Site == "" || scope.SchemaName == "" {
		return nil, ErrInvalidCursor
	}
	filter := any(nil)
	if scope.Filter != nil {
		var equals any
		decoder := json.NewDecoder(bytes.NewReader(scope.Filter.Equals))
		decoder.UseNumber()
		if err := decoder.Decode(&equals); err != nil {
			return nil, ErrInvalidCursor
		}
		filter = map[string]any{
			s.contract.ScopeFilterFieldName:  scope.Filter.Field,
			s.contract.ScopeFilterEqualsName: equals,
		}
	}
	values := map[string]any{
		s.contract.ScopeFields[0]: scope.Site,
		s.contract.ScopeFields[1]: scope.SchemaName,
		s.contract.ScopeFields[2]: filter,
	}
	canonical, err := json.Marshal(values)
	if err != nil {
		return nil, ErrInvalidCursor
	}
	digest := sha256.Sum256(canonical)
	return digest[:], nil
}

func loadCursorContract() (cursorContract, error) {
	var contract cursorContract
	if err := json.Unmarshal(cursorContractJSON, &contract); err != nil || contract.Version != 1 || contract.KeyBytes < 32 ||
		contract.GrantCapability == "" || contract.GrantPurpose == "" || contract.GrantDomain == "" ||
		contract.LifetimeSeconds < 1 || contract.DefaultPageSize < 1 || contract.MaxPageSize < contract.DefaultPageSize || contract.LookaheadRows < 1 ||
		contract.MaxTokenBytes < 1 || len(contract.ScopeFields) != 3 || contract.ScopeFields[0] == "" || contract.ScopeFields[1] == "" || contract.ScopeFields[2] == "" ||
		contract.ScopeFilterFieldName == "" || contract.ScopeFilterEqualsName == "" || len(contract.ClaimFields) != 5 ||
		contract.InternalInvalidCursor == "" || contract.InternalKeyUnavailable == "" {
		return cursorContract{}, ErrCursorKeyUnavailable
	}
	for _, key := range []string{"scopeDigest", "createdAt", "id", "issuedAt", "expiresAt"} {
		if contract.ClaimFields[key] == "" {
			return cursorContract{}, ErrCursorKeyUnavailable
		}
	}
	return contract, nil
}

func clear(value []byte) {
	for index := range value {
		value[index] = 0
	}
}
