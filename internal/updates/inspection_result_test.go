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

func TestInspectionIdentityRejectsOversizedInputsBeforeAllocation(t *testing.T) {
	base := InspectionIdentity{OperationID: strings.Repeat("a", 20), Release: "0", PackageSHA256: strings.Repeat("ab", 32), PackageLength: 1}
	for _, field := range []string{"operation", "release", "hash"} {
		oversized := base
		switch field {
		case "operation":
			oversized.OperationID = strings.Repeat("a", 1<<20)
		case "release":
			oversized.Release = strings.Repeat("0", 1<<20)
		case "hash":
			oversized.PackageSHA256 = strings.Repeat("a", 1<<20)
		}
		if validInspectionIdentity(oversized) {
			t.Fatal("oversized identity admitted", field)
		}
		if allocations := testing.AllocsPerRun(100, func() {
			if validInspectionIdentity(oversized) {
				panic("oversized input admitted")
			}
		}); allocations != 0 {
			t.Fatal("oversized identity allocated", field, allocations)
		}
	}
	for _, size := range []int{20, 64} {
		identity := base
		identity.OperationID = strings.Repeat("a", size)
		identity.Release = "0" + strings.Repeat("a", 63)
		identity.PackageLength = 512 << 20
		if !validInspectionIdentity(identity) {
			t.Fatal("supported identity boundary refused", size)
		}
	}
	for _, altered := range []InspectionIdentity{
		{OperationID: strings.Repeat("a", 19), Release: base.Release, PackageSHA256: base.PackageSHA256, PackageLength: 1},
		{OperationID: base.OperationID, Release: base.Release, PackageSHA256: base.PackageSHA256[:63], PackageLength: 1},
		{OperationID: base.OperationID, Release: base.Release, PackageSHA256: base.PackageSHA256, PackageLength: 0},
		{OperationID: base.OperationID, Release: base.Release, PackageSHA256: base.PackageSHA256, PackageLength: (512 << 20) + 1},
	} {
		if validInspectionIdentity(altered) {
			t.Fatal("invalid identity boundary admitted")
		}
	}
}
