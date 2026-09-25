package unit

import (
	"encoding/json"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestRuntimeContractValuesAreLoadedFromVersionedAssets(t *testing.T) {
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal("resolve plugin repository root")
	}
	contractPath := filepath.Join(root, "internal", "infrastructure", "security", "contracts", "cursor.json")
	data, err := os.ReadFile(contractPath)
	if err != nil {
		t.Fatal("read cursor security contract")
	}
	var contract map[string]any
	if err := json.Unmarshal(data, &contract); err != nil {
		t.Fatal("decode cursor security contract")
	}
	fields := []string{
		"keyFileEnvironment", "encryption", "encryptionKeyLabel", "authentication",
		"authenticationKeyLabel", "internalInvalidCursorError", "internalCursorKeyUnavailableError",
	}
	contractValues := make(map[string]struct{}, len(fields))
	for _, field := range fields {
		value, ok := contract[field].(string)
		if !ok || value == "" {
			t.Fatalf("cursor contract value %q is missing", field)
		}
		contractValues[value] = struct{}{}
	}

	var violations []string
	set := token.NewFileSet()
	err = filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			if entry.Name() == ".git" || entry.Name() == "vendor" {
				return filepath.SkipDir
			}
			return nil
		}
		if filepath.Ext(path) != ".go" || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		file, err := parser.ParseFile(set, path, nil, 0)
		if err != nil {
			return err
		}
		ast.Inspect(file, func(node ast.Node) bool {
			literal, ok := node.(*ast.BasicLit)
			if !ok || literal.Kind != token.STRING {
				return true
			}
			value, err := strconv.Unquote(literal.Value)
			if err != nil {
				return true
			}
			if _, found := contractValues[value]; found {
				position := set.Position(literal.Pos())
				violations = append(violations, position.String()+": duplicate cursor contract value")
			}
			return true
		})
		return nil
	})
	if err != nil {
		t.Fatal("scan production Go files")
	}
	if len(violations) != 0 {
		t.Fatalf("runtime contract values must live only in versioned assets:\n%s", strings.Join(violations, "\n"))
	}
}

func TestCursorSecurityAssetMatchesImplementedAlgorithms(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "internal", "infrastructure", "security", "contracts", "cursor.json"))
	if err != nil {
		t.Fatal("read cursor security contract")
	}
	var contract struct {
		Encryption     string `json:"encryption"`
		Authentication string `json:"authentication"`
	}
	if err := json.Unmarshal(data, &contract); err != nil {
		t.Fatal("decode cursor security contract")
	}
	if contract.Encryption != "AES-256-GCM" || contract.Authentication != "HMAC-SHA-256" {
		t.Fatalf("cursor contract algorithms do not match the implemented authenticated-encryption profile")
	}
}

func TestCursorSecurityAssetDocumentsWireFormatAndTimeUnits(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "internal", "infrastructure", "security", "contracts", "cursor.json"))
	if err != nil {
		t.Fatal("read cursor security contract")
	}
	var contract struct {
		TokenLayout           []string          `json:"tokenLayout"`
		ClaimsEncoding        string            `json:"claimsEncoding"`
		TimestampUnit         string            `json:"timestampUnit"`
		AuthenticatedBytes    string            `json:"authenticatedBytes"`
		AEADAdditionalData    string            `json:"aeadAdditionalData"`
		CiphertextFormat      string            `json:"ciphertextFormat"`
		ScopeDigestInput      string            `json:"scopeDigestInput"`
		ScopeCanonicalization string            `json:"scopeCanonicalization"`
		ScopeDigestEncoding   string            `json:"scopeDigestEncoding"`
		ClaimsSerialization   string            `json:"claimsSerialization"`
		ClaimSemantics        map[string]string `json:"claimSemantics"`
		ExpiryRule            string            `json:"expiryRule"`
		KeyDerivation         map[string]string `json:"keyDerivation"`
	}
	if err := json.Unmarshal(data, &contract); err != nil {
		t.Fatal("decode cursor security contract")
	}
	expectedLayout := []string{"version", "nonce", "ciphertext", "authenticator"}
	if strings.Join(contract.TokenLayout, ",") != strings.Join(expectedLayout, ",") ||
		contract.ClaimsEncoding != "объект JSON в UTF-8" || contract.TimestampUnit != "наносекунды Unix time" ||
		contract.AuthenticatedBytes != "version || nonce || ciphertext" || contract.AEADAdditionalData != "байт version" ||
		contract.CiphertextFormat != "шифротекст AES-GCM, затем добавленный GCM tag" ||
		contract.ScopeDigestInput != "канонический JSON-объект site, schemaName и filter; отсутствующий filter равен null; поля filter: field и equals" ||
		contract.ScopeCanonicalization != "filter.equals разбирается с JSON UseNumber; Go encoding/json сериализует scope компактно и сортирует ключи" ||
		contract.ScopeDigestEncoding != "base64url без padding" ||
		contract.ClaimsSerialization != "Go encoding/json сериализует claims компактно в UTF-8 и сортирует ключи объекта лексикографически" ||
		contract.ExpiryRule != "issuedAt <= now < expiresAt; expiresAt - issuedAt равно lifetimeSeconds после перевода в timestampUnit" {
		t.Fatal("cursor contract does not fully specify token framing, scope, or timestamp units")
	}
	for _, claim := range []string{"scopeDigest", "createdAt", "id", "issuedAt", "expiresAt"} {
		if contract.ClaimSemantics[claim] == "" {
			t.Fatalf("cursor claim semantics are missing for %q", claim)
		}
	}
	if contract.KeyDerivation["encryption"] == "" || contract.KeyDerivation["authentication"] == "" {
		t.Fatal("cursor contract does not specify key derivation")
	}
}
