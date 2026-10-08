package install

import (
	"bytes"
	"encoding/json"
	"io"
)

// validateActivationConditions admits the typed busctl Conditions property,
// not ConditionResult (which only describes a previous start). This is a
// snapshot component, never a retained runtime exclusion lease.
// systemd v255 busctl.c get_property/json_transform_variant emits this shape.
func validateActivationConditions(data []byte) error {
	if len(data) == 0 || len(data) > 8192 {
		return ErrConflict
	}
	d := json.NewDecoder(bytes.NewReader(data))
	start, err := d.Token()
	if err != nil || start != json.Delim('{') {
		return ErrConflict
	}
	seen := map[string]bool{}
	var signature string
	var conditions [][]json.RawMessage
	for d.More() {
		key, err := d.Token()
		name, ok := key.(string)
		if err != nil || !ok || seen[name] {
			return ErrConflict
		}
		seen[name] = true
		switch name {
		case "type":
			if d.Decode(&signature) != nil {
				return ErrConflict
			}
		case "data":
			if d.Decode(&conditions) != nil {
				return ErrConflict
			}
		default:
			return ErrConflict
		}
	}
	end, err := d.Token()
	if err != nil || end != json.Delim('}') || len(seen) != 2 || signature != "a(sbbsi)" || len(conditions) == 0 || len(conditions) > 16 {
		return ErrConflict
	}
	if _, err := d.Token(); err != io.EOF {
		return ErrConflict
	}
	guards := 0
	for _, c := range conditions {
		if len(c) != 5 {
			return ErrConflict
		}
		// Null cannot stand in for a typed D-Bus field.
		for _, field := range c {
			if bytes.Equal(bytes.TrimSpace(field), []byte("null")) {
				return ErrConflict
			}
		}
		var kind, path string
		var trigger, negate bool
		var previousResult int32
		if json.Unmarshal(c[0], &kind) != nil || json.Unmarshal(c[1], &trigger) != nil || json.Unmarshal(c[2], &negate) != nil || json.Unmarshal(c[3], &path) != nil || json.Unmarshal(c[4], &previousResult) != nil || trigger {
			return ErrConflict
		}
		if kind == "ConditionPathExists" && path == "/var/lib/homenode-install/recovery-blocked" {
			if !negate {
				return ErrConflict
			}
			guards++
		}
	}
	if guards != 1 {
		return ErrConflict
	}
	return nil
}
