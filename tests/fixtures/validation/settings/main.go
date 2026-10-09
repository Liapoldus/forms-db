package main

import (
	"encoding/json"
	"fmt"
	"github.com/Liapoldus/forms-db/tests/fixtures/support"

	"github.com/Liapoldus/forms-db/internal/infrastructure/config"
)

func main() {
	cases := map[string][]byte{
		"valid":             []byte(`{"driver":"memory","schemas":{}}`),
		"duplicateTopLevel": []byte(`{"driver":"memory","driver":"sqlite","dsn":"file:storage","schemas":{}}`),
		"duplicateNested":   []byte(`{"driver":"memory","schemas":{"contact":{"type":"object","properties":{"email":{"type":"string","type":"number"}}}}}`),
		"trailingDocument":  []byte(`{"driver":"memory","schemas":{}} {}`),
		"unknownTopLevel":   []byte(`{"driver":"memory","schemas":{},"unexpected":true}`),
		"invalidUTF8":       {'{', '}', 0xff},
	}
	results := make(map[string]bool, len(cases))
	for name, raw := range cases {
		_, err := config.Apply(raw)
		results[name] = err == nil
	}
	encoded, err := json.Marshal(results)
	if err != nil {
		panic(err)
	}
	support.Written(fmt.Println(string(encoded)))
}
