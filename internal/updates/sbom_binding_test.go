package updates

import (
	"strings"
	"testing"
)

func TestSBOMPrimaryComponentBinding(t *testing.T) {
	hash := strings.Repeat("ab", 32)
	valid := `{"bomFormat":"CycloneDX","specVersion":"1.6","metadata":{"component":{"type":"application","name":"homenode","version":"0.1.0","hashes":[{"alg":"SHA-256","content":"` + hash + `"}]}}}`
	if err := ValidateSBOMBinding([]byte(valid), hash, "0.1.0"); err != nil {
		t.Fatal(err)
	}
	for _, bad := range []string{
		strings.Replace(valid, hash, strings.Repeat("cd", 32), 1),
		strings.Replace(valid, "0.1.0", "0.2.0", 1),
		strings.Replace(valid, "homenode", "foreign", 1),
		strings.Replace(valid, "metadata", "Metadata", 1),
		strings.Replace(valid, "1.6", "1.5", 1),
		strings.Replace(valid, `"hashes":[`, `"hashes":null,"ignored":[`, 1),
		strings.Replace(valid, `"bomFormat":"CycloneDX"`, `"bomFormat":"CycloneDX","bomFormat":"CycloneDX"`, 1),
		strings.Replace(valid, `"hashes":[`, `"hashes":[{"alg":"SHA-256","content":"`+hash+`"},`, 1),
	} {
		if err := ValidateSBOMBinding([]byte(bad), hash, "0.1.0"); err == nil {
			t.Fatal("unbound SBOM accepted", bad)
		}
	}
	if err := ValidateSBOMBinding([]byte(valid), strings.ToUpper(hash), "0.1.0"); err == nil {
		t.Fatal("noncanonical retained digest accepted")
	}
}
