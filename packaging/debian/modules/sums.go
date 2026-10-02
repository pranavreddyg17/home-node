package main

import (
	"encoding/base64"
	"fmt"
	"strings"
)

// Main module/source provenance is separate. This checks third-party module sums.
func verifyModuleSums(records []binaryRecord, data []byte) error {
	if len(data) > 8<<20 {
		return fmt.Errorf("oversized module sum inventory")
	}
	trusted := map[string]string{}
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}
		if len(fields) != 3 {
			return fmt.Errorf("invalid module sum record")
		}
		key := fields[0] + " " + fields[1]
		if _, exists := trusted[key]; exists {
			return fmt.Errorf("duplicate module sum record")
		}
		trusted[key] = fields[2]
	}
	for _, binary := range records {
		for _, original := range binary.Dependencies {
			if original == nil {
				return fmt.Errorf("missing compiled module identity")
			}
			module := original
			if module.Replace != nil {
				module = module.Replace
			}
			if module.Path == "" || module.Version == "" || module.Version == "(devel)" {
				return fmt.Errorf("unversioned compiled dependency")
			}
			if !strings.HasPrefix(module.Sum, "h1:") {
				return fmt.Errorf("missing compiled module source sum")
			}
			digest, err := base64.StdEncoding.Strict().DecodeString(strings.TrimPrefix(module.Sum, "h1:"))
			if err != nil || len(digest) != 32 || "h1:"+base64.StdEncoding.EncodeToString(digest) != module.Sum {
				return fmt.Errorf("noncanonical compiled module source sum")
			}
			if trusted[module.Path+" "+module.Version] != module.Sum {
				return fmt.Errorf("compiled source sum differs from reviewed inventory: %s", module.Path)
			}
		}
	}
	return nil
}
