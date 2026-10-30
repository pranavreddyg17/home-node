package supervisor

import (
	"bytes"
	"strings"
	"unicode/utf8"
)

// Launch-argument evidence only: argv is mutable process memory and is not
// attestation of the installed seccomp program. Never log the full command line.
func validateQEMUSandboxArguments(data []byte) error {
	if len(data) == 0 || len(data) > 128<<10 || data[len(data)-1] != 0 || !utf8.Valid(data) {
		return ErrPolicy
	}
	args := strings.Split(string(data[:len(data)-1]), "\x00")
	if len(args) < 3 || len(args) > 1024 || args[0] != "/usr/bin/qemu-system-x86_64" {
		return ErrPolicy
	}
	if bytes.ContainsAny(data, "\r\n") {
		return ErrPolicy
	}
	found := false
	for i := 1; i < len(args); i++ {
		if !strings.HasPrefix(args[i], "-sandbox") && !strings.HasPrefix(args[i], "--sandbox") {
			continue
		}
		if found || args[i] != "-sandbox" || i+1 >= len(args) {
			return ErrPolicy
		}
		found = true
		fields := strings.Split(args[i+1], ",")
		expected := map[string]bool{"on": true, "obsolete=deny": true, "elevateprivileges=deny": true, "spawn=deny": true, "resourcecontrol=deny": true}
		if len(fields) != len(expected) {
			return ErrPolicy
		}
		for _, field := range fields {
			if !expected[field] {
				return ErrPolicy
			}
			delete(expected, field)
		}
		if len(expected) != 0 {
			return ErrPolicy
		}
		i++
	}
	if !found {
		return ErrPolicy
	}
	return nil
}
