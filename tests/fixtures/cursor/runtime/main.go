package main

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"github.com/Liapoldus/forms-db/tests/fixtures/support"
	"time"

	"github.com/Liapoldus/forms-db/internal/domain/models"
	"github.com/Liapoldus/forms-db/internal/infrastructure/security"
)

func main() {
	issuedAt := time.Date(2026, time.October, 3, 0, 0, 0, 0, time.UTC)
	scope := security.CursorScope{Site: "portal", SchemaName: "contact"}
	position := models.SubmissionCursor{CreatedAt: "2026-10-03T00:00:00Z", ID: "frm_1"}
	firstKey := sha256.Sum256([]byte("forms-db cursor key one"))
	secondKey := sha256.Sum256([]byte("forms-db cursor key two"))
	signer, err := security.NewCursorSigner(firstKey[:])
	check(err)
	defer signer.Close()
	token, err := signer.Encode(scope, position, issuedAt)
	check(err)
	_, validErr := signer.Decode(token, scope, issuedAt.Add(time.Second))
	_, wrongScopeErr := signer.Decode(token, security.CursorScope{Site: "other", SchemaName: "contact"}, issuedAt.Add(time.Second))
	_, expiredErr := signer.Decode(token, scope, issuedAt.Add(24*time.Hour))
	rotated, err := security.NewCursorSigner(secondKey[:])
	check(err)
	_, rotatedErr := rotated.Decode(token, scope, issuedAt.Add(time.Second))
	rotated.Close()

	encoded, err := base64.RawURLEncoding.DecodeString(token)
	check(err)
	encoded[0]++
	_, versionErr := signer.Decode(base64.RawURLEncoding.EncodeToString(encoded), scope, issuedAt.Add(time.Second))
	encoded, err = base64.RawURLEncoding.DecodeString(token)
	check(err)
	encoded[len(encoded)/2] ^= 1
	_, tamperedErr := signer.Decode(base64.RawURLEncoding.EncodeToString(encoded), scope, issuedAt.Add(time.Second))

	output, err := json.Marshal(map[string]bool{
		"valid":                       validErr == nil,
		"scopeBound":                  wrongScopeErr != nil,
		"expires":                     expiredErr != nil,
		"rotationInvalidatesOldToken": rotatedErr != nil,
		"versionTamperingRejected":    versionErr != nil,
		"ciphertextTamperingRejected": tamperedErr != nil,
	})
	check(err)
	support.Written(fmt.Println(string(output)))
}

func check(err error) {
	if err != nil {
		panic(err)
	}
}
