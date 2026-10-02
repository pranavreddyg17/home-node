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
	for line := range strings.Lines(string(data)) {
		if len(line) > 8192 || len(trusted) >= 65536 {
			return fmt.Errorf("module sum inventory structure limit")
		}
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
	packageIdentities := map[string]string{}
	for _, binary := range records {
		if len(binary.Dependencies) > 4096 {
			return fmt.Errorf("oversized compiled module inventory")
		}
		seen := map[string]bool{}
		for _, original := range binary.Dependencies {
			if original == nil {
				return fmt.Errorf("missing compiled module identity")
			}
			if original.Path == "" || len(original.Path) > 2048 || seen[original.Path] {
				return fmt.Errorf("ambiguous compiled module identity")
			}
			seen[original.Path] = true
			module := original
			if module.Replace != nil {
				module = module.Replace
			}
			if module.Replace != nil || len(module.Path) > 2048 || len(module.Version) > 256 {
				return fmt.Errorf("invalid compiled replacement identity")
			}
			if module.Path == "" || module.Version == "" || module.Version == "(devel)" {
				return fmt.Errorf("unversioned compiled dependency")
			}
			if len(module.Sum) != 47 || !strings.HasPrefix(module.Sum, "h1:") {
				return fmt.Errorf("missing compiled module source sum")
			}
			digest, err := base64.StdEncoding.Strict().DecodeString(strings.TrimPrefix(module.Sum, "h1:"))
			if err != nil || len(digest) != 32 || "h1:"+base64.StdEncoding.EncodeToString(digest) != module.Sum {
				return fmt.Errorf("noncanonical compiled module source sum")
			}
			identity := module.Path + " " + module.Version + " " + module.Sum
			if prior, exists := packageIdentities[original.Path]; exists && prior != identity {
				return fmt.Errorf("inconsistent packaged dependency identity: %s", original.Path)
			}
			packageIdentities[original.Path] = identity
			if trusted[module.Path+" "+module.Version] != module.Sum {
				return fmt.Errorf("compiled source sum differs from reviewed inventory: %s", module.Path)
			}
		}
	}
	return nil
}
