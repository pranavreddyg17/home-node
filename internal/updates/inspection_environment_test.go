package updates

import (
	"strings"
	"testing"
)

func TestInspectionEnvironmentRejectsInjectedLaunchInputs(t *testing.T) {
	identity := InspectionIdentity{OperationID: "inspection-fixture-000001", Release: "0.1.0~ci+build-1", PackageSHA256: strings.Repeat("a", 64), PackageLength: 100}
	environment, err := InspectionEnvironment(identity)
	if err != nil || string(environment) != "INSPECTION_RELEASE=0.1.0~ci+build-1\nINSPECTION_OPERATION_ID=inspection-fixture-000001\n" {
		t.Fatal("unexpected service configuration", string(environment), err)
	}
	for _, value := range []string{"0.1\nOTHER=value", "0.1\rOTHER=value", "0.1 ${HOME}", "0.1%u", "0.1'", "0.1\"", "0.1\\", "0.1\x00", "0.1;command", "0.1$(command)"} {
		changed := identity
		changed.Release = value
		if _, err := InspectionEnvironment(changed); err == nil {
			t.Fatalf("release injection accepted: %q", value)
		}
		changed = identity
		changed.OperationID = identity.OperationID + value
		if _, err := InspectionEnvironment(changed); err == nil {
			t.Fatalf("operation injection accepted: %q", value)
		}
	}
	for _, modify := range []func(*InspectionIdentity){func(i *InspectionIdentity) { i.PackageSHA256 = strings.Repeat("A", 64) }, func(i *InspectionIdentity) { i.PackageLength = 0 }, func(i *InspectionIdentity) { i.PackageLength = 513 << 20 }} {
		changed := identity
		modify(&changed)
		if _, err := InspectionEnvironment(changed); err == nil {
			t.Fatal("invalid retained identity accepted")
		}
	}
}
