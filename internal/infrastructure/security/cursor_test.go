package security

import (
	"bytes"
	"errors"
	"testing"
	"time"

	"github.com/Liapoldus/forms-db/internal/domain/models"
)

func TestCursorAuthenticatesOpaqueScopedClaims(t *testing.T) {
	signer := testSigner(t)
	scope := CursorScope{Site: "portal", SchemaName: "contact", Filter: &models.SubmissionFilter{Field: "email", Equals: []byte(`"a@example.test"`)}}
	position := models.SubmissionCursor{CreatedAt: "2026-01-02T00:00:00Z", ID: "frm_cursor_position"}
	issued := time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)
	token, err := signer.Encode(scope, position, issued)
	if err != nil {
		t.Fatal("encode cursor failed")
	}
	if bytes.Contains([]byte(token), []byte(position.ID)) || bytes.Contains([]byte(token), []byte(scope.Filter.Equals)) {
		t.Fatal("cursor must not disclose position or filter internals")
	}
	decoded, err := signer.Decode(token, scope, issued.Add(time.Minute))
	if err != nil || decoded != position {
		t.Fatalf("valid cursor did not round-trip: position=%+v err=%v", decoded, err)
	}
	wrongScope := scope
	wrongScope.Site = "other"
	assertInvalidCursor(t, signer, token, wrongScope, issued.Add(time.Minute))

	tampered := []byte(token)
	if tampered[len(tampered)-1] == 'A' {
		tampered[len(tampered)-1] = 'B'
	} else {
		tampered[len(tampered)-1] = 'A'
	}
	assertInvalidCursor(t, signer, string(tampered), scope, issued.Add(time.Minute))
	assertInvalidCursor(t, signer, token, scope, issued.Add(16*time.Minute))
}

func TestCursorRejectsEmptyOrShortSecret(t *testing.T) {
	for _, key := range [][]byte{nil, {}, []byte("too-short")} {
		if _, err := NewCursorSigner(key); err == nil {
			t.Fatal("cursor signer must reject missing or undersized key material")
		}
	}
}

func TestInvalidCursorErrorDoesNotContainToken(t *testing.T) {
	signer := testSigner(t)
	token := "sensitive-cursor-token"
	_, err := signer.Decode(token, CursorScope{Site: "portal", SchemaName: "contact"}, time.Now())
	if err == nil || errors.Is(err, ErrCursorKeyUnavailable) {
		t.Fatal("malformed cursor must be rejected")
	}
	if bytes.Contains([]byte(err.Error()), []byte(token)) {
		t.Fatal("cursor error must not expose token contents")
	}
}

func testSigner(t *testing.T) *CursorSigner {
	t.Helper()
	signer, err := NewCursorSigner(bytes.Repeat([]byte{0x5a}, 32))
	if err != nil {
		t.Fatal("construct test cursor signer")
	}
	return signer
}

func assertInvalidCursor(t *testing.T, signer *CursorSigner, token string, scope CursorScope, now time.Time) {
	t.Helper()
	if _, err := signer.Decode(token, scope, now); !errors.Is(err, ErrInvalidCursor) {
		t.Fatalf("cursor must be rejected without exposing details: err=%v", err)
	}
}
