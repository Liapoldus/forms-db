package unit

import (
	"bytes"
	"crypto/rand"
	"errors"
	"testing"
	"time"

	"github.com/Liapoldus/forms-db/internal/domain/models"
	"github.com/Liapoldus/forms-db/internal/infrastructure/security"
)

func TestCursorAuthenticatesOpaqueScopedClaims(t *testing.T) {
	signer := testSigner(t)
	scope := security.CursorScope{Site: "portal", SchemaName: "contact", Filter: &models.SubmissionFilter{Field: "email", Equals: []byte(`"a@example.test"`)}}
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
	tamperAt := len(tampered) / 2
	if tampered[tamperAt] == 'A' {
		tampered[tamperAt] = 'B'
	} else {
		tampered[tamperAt] = 'A'
	}
	assertInvalidCursor(t, signer, string(tampered), scope, issued.Add(time.Minute))
	assertInvalidCursor(t, signer, token, scope, issued.Add(16*time.Minute))
}

func TestCursorRejectsEmptyOrShortSecret(t *testing.T) {
	for _, key := range [][]byte{nil, {}, []byte("too-short")} {
		if _, err := security.NewCursorSigner(key); err == nil {
			t.Fatal("cursor signer must reject missing or undersized key material")
		}
	}
}

func TestInvalidCursorErrorDoesNotContainToken(t *testing.T) {
	signer := testSigner(t)
	token := "sensitive-cursor-token"
	_, err := signer.Decode(token, security.CursorScope{Site: "portal", SchemaName: "contact"}, time.Now())
	if err == nil || errors.Is(err, security.ErrCursorKeyUnavailable) {
		t.Fatal("malformed cursor must be rejected")
	}
	if bytes.Contains([]byte(err.Error()), []byte(token)) {
		t.Fatal("cursor error must not expose token contents")
	}
}

func testSigner(t *testing.T) *security.CursorSigner {
	t.Helper()
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		t.Fatal("generate ephemeral cursor key")
	}
	defer clear(key)
	signer, err := security.NewCursorSigner(key)
	if err != nil {
		t.Fatal("construct test cursor signer")
	}
	return signer
}

func assertInvalidCursor(t *testing.T, signer *security.CursorSigner, token string, scope security.CursorScope, now time.Time) {
	t.Helper()
	if _, err := signer.Decode(token, scope, now); !errors.Is(err, security.ErrInvalidCursor) {
		t.Fatalf("cursor must be rejected without exposing details: err=%v", err)
	}
}
