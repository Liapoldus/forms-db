package peerplugin

import (
	"bytes"

	"encoding/json"
	"errors"
	"io"
	"strings"

	"github.com/Liapoldus/pluginprotocol/v2/presentation/peer"
)

func requestPayload(payload []byte) ([]byte, error) {
	// HTTP context is transport metadata owned by the caller. Preserve opaque
	// context fields such as allow-listed cookies while extracting only Body;
	// product decoders must never interpret or persist those values.
	var envelope struct {
		Method     string            `json:"method"`
		Path       string            `json:"path"`
		Query      string            `json:"query"`
		Headers    map[string]string `json:"headers"`
		Cookies    json.RawMessage   `json:"cookies"`
		Body       []byte            `json:"body"`
		RequestID  string            `json:"requestId"`
		RemoteAddr string            `json:"remoteAddr"`
	}
	if decodeObject(payload, &envelope) == nil && envelope.Body != nil {
		return envelope.Body, nil
	}
	return payload, nil
}

func decodeObject(payload []byte, target any) error {
	trimmed := bytes.TrimSpace(payload)
	if len(trimmed) == 0 || trimmed[0] != '{' {
		return peer.ErrInvalidRequest
	}
	if err := rejectDuplicateKeys(trimmed); err != nil {
		return peer.ErrInvalidRequest
	}
	decoder := json.NewDecoder(bytes.NewReader(trimmed))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return peer.ErrInvalidRequest
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return peer.ErrInvalidRequest
	}
	return nil
}

func rejectDuplicateKeys(document []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(document))
	first, err := decoder.Token()
	if err != nil {
		return err
	}
	if err := scanJSONValue(decoder, first, true); err != nil {
		return err
	}
	if _, err := decoder.Token(); !errors.Is(err, io.EOF) {
		return peer.ErrInvalidRequest
	}
	return nil
}

func scanJSONValue(decoder *json.Decoder, token json.Token, root bool) error {
	delimiter, ok := token.(json.Delim)
	if !ok {
		return nil
	}
	switch delimiter {
	case '{':
		keys := make(map[string]struct{})
		rootKeys := make(map[string]string)
		for decoder.More() {
			keyToken, err := decoder.Token()
			if err != nil {
				return err
			}
			key, ok := keyToken.(string)
			if !ok {
				return peer.ErrInvalidRequest
			}
			if _, exists := keys[key]; exists {
				return peer.ErrInvalidRequest
			}
			keys[key] = struct{}{}
			if root {
				for existing := range rootKeys {
					if strings.EqualFold(existing, key) {
						return peer.ErrInvalidRequest
					}
				}
				rootKeys[key] = key
			}
			value, err := decoder.Token()
			if err != nil {
				return err
			}
			if err := scanJSONValue(decoder, value, false); err != nil {
				return err
			}
		}
		closing, err := decoder.Token()
		if err != nil || closing != json.Delim('}') {
			return peer.ErrInvalidRequest
		}
	case '[':
		for decoder.More() {
			value, err := decoder.Token()
			if err != nil {
				return err
			}
			if err := scanJSONValue(decoder, value, false); err != nil {
				return err
			}
		}
		closing, err := decoder.Token()
		if err != nil || closing != json.Delim(']') {
			return peer.ErrInvalidRequest
		}
	default:
		return peer.ErrInvalidRequest
	}
	return nil
}

func httpJSON(status int, value any) peer.Result {
	body, err := json.Marshal(value)
	if err != nil {
		panic("invalid forms response")
	}
	payload, err := json.Marshal(map[string]any{
		"status":  status,
		"headers": map[string]string{"Content-Type": "application/json"},
		"body":    string(body),
	})
	if err != nil {
		panic("invalid forms envelope")
	}
	return peer.Result{Payload: payload}
}
