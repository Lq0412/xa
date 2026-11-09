package run

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
)

func argsHash(name string, args []byte) string {
	norm, err := canonicalJSON(args)
	if err != nil {
		norm = args
	}
	payload, _ := json.Marshal(struct {
		Name string          `json:"name"`
		Args json.RawMessage `json:"args"`
	}{Name: name, Args: norm})
	sum := sha256.Sum256(payload)
	return hex.EncodeToString(sum[:])
}

func canonicalJSON(raw []byte) ([]byte, error) {
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		return nil, err
	}
	out, err := json.Marshal(sortJSON(v))
	if err != nil {
		return nil, err
	}
	return out, nil
}

func sortJSON(v any) any {
	switch x := v.(type) {
	case map[string]any:
		keys := make([]string, 0, len(x))
		for k := range x {
			keys = append(keys, k)
		}
		for i := 0; i < len(keys); i++ {
			for j := i + 1; j < len(keys); j++ {
				if keys[j] < keys[i] {
					keys[i], keys[j] = keys[j], keys[i]
				}
			}
		}
		m := make(map[string]any, len(x))
		for _, k := range keys {
			m[k] = sortJSON(x[k])
		}
		return m
	case []any:
		for i := range x {
			x[i] = sortJSON(x[i])
		}
		return x
	default:
		return v
	}
}
