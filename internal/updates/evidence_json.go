package updates

import (
	"bytes"
	"encoding/json"
	"io"
	"unicode/utf8"
)

// validEvidenceJSON rejects ambiguous evidence before later semantic review.
// It establishes bounded object syntax, not SBOM or provenance qualification.
func validEvidenceJSON(data []byte) bool {
	if len(data) == 0 || len(data) > 8<<20 || !utf8.Valid(data) {
		return false
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	tokens := 0
	var value func(int, bool) bool
	value = func(depth int, root bool) bool {
		if depth > 64 || tokens >= 65536 {
			return false
		}
		token, err := decoder.Token()
		tokens++
		if err != nil {
			return false
		}
		delimiter, container := token.(json.Delim)
		if !container {
			return !root
		}
		switch delimiter {
		case '{':
			seen := map[string]bool{}
			for decoder.More() {
				if tokens >= 65536 {
					return false
				}
				key, err := decoder.Token()
				tokens++
				name, ok := key.(string)
				if err != nil || !ok || seen[name] {
					return false
				}
				seen[name] = true
				if !value(depth+1, false) {
					return false
				}
			}
			end, err := decoder.Token()
			tokens++
			return err == nil && tokens <= 65536 && end == json.Delim('}')
		case '[':
			if root {
				return false
			}
			for decoder.More() {
				if !value(depth+1, false) {
					return false
				}
			}
			end, err := decoder.Token()
			tokens++
			return err == nil && tokens <= 65536 && end == json.Delim(']')
		}
		return false
	}
	if !value(1, true) {
		return false
	}
	// Probe only the next token: decoding an entire trailing value would parse
	// a second document outside the structural budget before refusing it.
	_, err := decoder.Token()
	return err == io.EOF
}
