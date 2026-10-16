package control

import (
	"bytes"
	"encoding/json"
	"io"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Standard struct decoding accepts duplicate/case-aliased fields and replaces
// invalid UTF-8. Reject ambiguous input before decoding any application value.
func unambiguousRequestObject(body []byte) bool {
	if !utf8.Valid(body) {
		return false
	}
	trimmed := bytes.TrimSpace(body)
	if len(trimmed) == 0 || trimmed[0] != '{' {
		return false
	}
	d := json.NewDecoder(bytes.NewReader(body))
	d.UseNumber()
	var value func(int) bool
	value = func(depth int) bool {
		if depth > 32 {
			return false
		}
		token, err := d.Token()
		if err != nil {
			return false
		}
		delim, compound := token.(json.Delim)
		if !compound {
			return true
		}
		switch delim {
		case '{':
			seen := map[string]bool{}
			for d.More() {
				if len(seen) >= 128 {
					return false
				}
				key, err := d.Token()
				name, ok := key.(string)
				if err != nil || !ok {
					return false
				}
				folded := foldedRequestMember(name)
				if seen[folded] {
					return false
				}
				seen[folded] = true
				if !value(depth + 1) {
					return false
				}
			}
			end, err := d.Token()
			return err == nil && end == json.Delim('}')
		case '[':
			for d.More() {
				if !value(depth + 1) {
					return false
				}
			}
			end, err := d.Token()
			return err == nil && end == json.Delim(']')
		default:
			return false
		}
	}
	if !value(0) {
		return false
	}
	_, err := d.Token()
	return err == io.EOF
}

// Match encoding/json's Unicode case equivalence without quadratic pairwise
// comparisons between attacker-controlled field names.
func foldedRequestMember(name string) string {
	var key strings.Builder
	for _, r := range name {
		minimum := r
		for next := unicode.SimpleFold(r); next != r; next = unicode.SimpleFold(next) {
			if next < minimum {
				minimum = next
			}
		}
		key.WriteRune(minimum)
	}
	return key.String()
}
