package guest

import (
	"encoding/json"
	"io"
	"strings"
	"unicode/utf8"
)

// validModelJSON rejects ambiguous keys before the typed decoder discards them.
// Input is already limited by the SSE scanner; depth also bounds recursion.
func validModelJSON(value string) bool {
	if !utf8.ValidString(value) {
		return false
	}
	decoder := json.NewDecoder(strings.NewReader(value))
	decoder.UseNumber()
	var read func(int) bool
	read = func(depth int) bool {
		if depth > 32 {
			return false
		}
		token, err := decoder.Token()
		if err != nil {
			return false
		}
		delim, compound := token.(json.Delim)
		if !compound {
			return true
		}
		switch delim {
		case '{':
			keys := map[string]bool{}
			for decoder.More() {
				token, err := decoder.Token()
				key, ok := token.(string)
				if err != nil || !ok || keys[key] {
					return false
				}
				keys[key] = true
				if !read(depth + 1) {
					return false
				}
			}
			end, err := decoder.Token()
			return err == nil && end == json.Delim('}')
		case '[':
			for decoder.More() {
				if !read(depth + 1) {
					return false
				}
			}
			end, err := decoder.Token()
			return err == nil && end == json.Delim(']')
		default:
			return false
		}
	}
	if !read(0) {
		return false
	}
	_, err := decoder.Token()
	return err == io.EOF
}
