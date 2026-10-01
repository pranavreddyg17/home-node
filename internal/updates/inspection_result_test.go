package updates

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestInspectionResultRequiresExactIdentityAndNoInstallAuthority(t *testing.T) {
	identity := InspectionIdentity{OperationID: "inspection-fixture-000001", Release: "0.1.0", PackageSHA256: strings.Repeat("ab", 32), PackageLength: 123}
	valid := InspectionResult{OperationID: identity.OperationID, Schema: 1, Release: identity.Release, PackageSHA256: identity.PackageSHA256, PackageLength: identity.PackageLength, ContentValid: true}
	data, err := json.Marshal(valid)
	if err != nil {
		t.Fatal(err)
	}
	if result, err := ValidateInspectionResult(data, identity); err != nil || result != valid {
		t.Fatal(result, err)
	}
	for _, scenario := range []string{"operation", "schema", "release", "hash", "length", "failed", "install-authority"} {
		result := valid
		switch scenario {
		case "operation":
			result.OperationID = "inspection-fixture-000002"
		case "schema":
			result.Schema = 2
		case "release":
			result.Release = "0.2.0"
		case "hash":
			result.PackageSHA256 = strings.Repeat("cd", 32)
		case "length":
			result.PackageLength++
		case "failed":
			result.ContentValid = false
		case "install-authority":
			result.InstallAuthorized = true
		}
		changed, err := json.Marshal(result)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := ValidateInspectionResult(changed, identity); err == nil {
			t.Fatal("mismatched result accepted", scenario)
		}
	}
	text := string(data)
	for _, changed := range []string{
		text + "{}", `{}`, `null`, strings.Repeat(" ", 2049) + text,
		strings.Replace(text, `"schema":1`, `"schema":1,"schema":1`, 1),
		strings.Replace(text, `"schema":1`, `"schema":1,"sc\u0068ema":1`, 1),
		strings.Replace(text, `"schema":1`, `"Schema":1`, 1),
		strings.Replace(text, `"schema":1`, `"schema":1,"extra":true`, 1),
		strings.Replace(text, `"installAuthorized":false`, `"installAuthorized":null`, 1),
		strings.Replace(text, `,"installAuthorized":false`, "", 1),
	} {
		if _, err := ValidateInspectionResult([]byte(changed), identity); err == nil {
			t.Fatal("ambiguous result accepted")
		}
	}
}
