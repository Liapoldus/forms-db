package contracts_test

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"github.com/Liapoldus/forms-db/contracts"
	"testing"
)

// Pre-cleanup fingerprints protect product fields, constraints and fixture vectors.
func TestContractSemanticsUnchanged(t *testing.T) {
	expected := map[string]string{ // #nosec G101 -- Public pre-cleanup SHA-256 fingerprints, not credentials.
		"v1/admin-surface-request.schema.json":  "b8f24894fdd4261b95310dcaed298e03c402528d19120a52a23cecc19b46eb36",
		"v1/admin-surface-response.schema.json": "69bc6893d175fa6b7b0e54d8c37169e2edc7d67aadcb7cb2edfde8f05f08277e",
		"v1/admin-surface-vectors.json":         "6237d4208a7ed69b6c1903b6545f5d0491616347b9722769abc6b1f142651689",
		"v1/admin-surface.json":                 "967dcab8fbaeec94bf27d5877c3b8f703c7bc2123cf18a97853c8258737e916f",
		"v1/cursor-replica-vectors.json":        "b064140ee34b6291a0d1e353733a2d42bba14fa7d2f19be76740a703f0e37112",
		"v1/cursor.json":                        "65128fabc1ac7348c10935cfdd9fdcbe046107d79b94d3c599f515c66d43c5a3",
		"v1/delete-errors.json":                 "9836621379eb6ee6b024a6845a5feff58d9cfa070efb2b65267f041a19f876c3",
		"v1/delete-negative-vectors.json":       "3fce2614d1dc80d006f455e52331afb8a17ae3caa0e6dbbb35bf64937f74d66b",
		"v1/delete-request.schema.json":         "4a3a364fdd7d81e183e898f9c84e3c0acd9050aa096a0cac6e2431553bc04843",
		"v1/delete-response.schema.json":        "59aece63507892a7699244ad80ccb7bac2fd6a6ac1d53c1cdcf1378b9be17b16",
		"v1/list-errors.json":                   "fc64f47bdcf6dcaf0c652df254faf49420e31b6206a80bfa496ee6903994ed42",
		"v1/list-request.schema.json":           "8c2eb1dcd9f6b27d32b093bb5aa63bbedb0705461b23920a6d19548dc2a7f393",
		"v1/list-response.schema.json":          "f906bc118e9de4d9ac0ee3a74814b1fb76d49a30764614eb17a595a173cf097c",
		"v1/plugin.json":                        "b99de3a8f815dcd10bdb9bdebb7cc132da2db91f77a13e9db1c3a8c6ae13d05f",
		"v1/request-json-vectors.json":          "56d75959a9197d999e0f50b9c20722a4a04a97db4d584cdc940ccece5dff75ae",
		"v1/runtime-limits.json":                "ca39ebce0ffb69913e5e5773cfd561656b44dd81b51d5ddd0368e9eb890302d1",
		"v1/secrets.json":                       "fa5f7fe1864b90b0361cffaa4fa8aa00888eeab0be967f23e3471ee38fe89922",
		"v1/settings.schema.json":               "5f18b266a833ca32e636bd6554a283132bcf440252130c1486853e4be580d3bf",
		"v1/submit-errors.json":                 "17000058535385b4819b1696e1a31b5664a375c3144f7b825bd4be6806212fa7",
		"v1/submit-negative-vectors.json":       "f3b2a5ed5fcddc291c2de7da4bcc1c6138a6c3059517ea7573dc0c11ab1e7fd7",
		"v1/submit-request.schema.json":         "5d51f3f54f9ba386a8f3f5886a5c588070dd417c81b13b5512f0323b65a12834",
		"v1/submit-response.schema.json":        "d8f9725847876ee3a9f62021333e8e493a2b9a0b8beba459536daa307eaeed74",
		"v1/validation.json":                    "abbc8004ebc746ae491fa257dc56df4dc0b0b55a26ba8be86963291f06182662",
	}
	for name, fingerprint := range expected {
		contents, err := contracts.Document(name)
		if err != nil {
			t.Fatal(err)
		}
		var document any
		if err := json.Unmarshal(contents, &document); err != nil {
			t.Fatal(err)
		}
		canonical, err := json.Marshal(document)
		if err != nil {
			t.Fatal(err)
		}
		if got := fmt.Sprintf("%x", sha256.Sum256(canonical)); got != fingerprint {
			t.Errorf("%s: contract semantics changed: %s", name, got)
		}
	}
}
