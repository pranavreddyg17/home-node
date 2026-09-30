package hostcheck

import (
	"testing"
	"time"
)

func TestEvaluateSupportedHostStillCannotExecute(t *testing.T) {
	report := Evaluate(Facts{OS: "linux", Architecture: "amd64", Distribution: "ubuntu", DistributionVersion: "24.04", KVMUsable: true, AppArmorEnforcing: true, LibvirtReachable: true, MemoryBytes: 16 * gib, AvailableDiskBytes: 80 * gib, DiskProbePath: "/var/lib"}, time.Unix(0, 0))
	if !report.PrerequisitesMet || report.ExecutionEnabled {
		t.Fatalf("preflight and execution must remain separate: %+v", report)
	}
}

func TestEvaluateRejectsMissingBoundaryAndResources(t *testing.T) {
	tests := []struct {
		name string
		edit func(*Facts)
		want string
	}{
		{"wrong OS", func(f *Facts) { f.OS = "darwin" }, "os"},
		{"wrong release", func(f *Facts) { f.DistributionVersion = "22.04" }, "distribution"},
		{"KVM missing", func(f *Facts) { f.KVMUsable = false }, "kvm"},
		{"AppArmor missing", func(f *Facts) { f.AppArmorEnforcing = false }, "apparmor"},
		{"libvirt missing", func(f *Facts) { f.LibvirtReachable = false }, "libvirt"},
		{"low memory", func(f *Facts) { f.MemoryBytes = 4 * gib }, "memory"},
		{"disk unknown", func(f *Facts) { f.AvailableDiskBytes = 0 }, "disk"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			f := Facts{OS: "linux", Architecture: "amd64", Distribution: "ubuntu", DistributionVersion: "24.04", KVMUsable: true, AppArmorEnforcing: true, LibvirtReachable: true, MemoryBytes: 16 * gib, AvailableDiskBytes: 80 * gib, DiskProbePath: "/var/lib"}
			tc.edit(&f)
			report := Evaluate(f, time.Now())
			if report.PrerequisitesMet {
				t.Fatal("unsupported host passed preflight")
			}
			found := false
			for _, check := range report.Checks {
				if check.ID == tc.want && check.Status == Fail && check.Remediation != "" {
					found = true
				}
			}
			if !found {
				t.Fatalf("missing actionable failure %s: %+v", tc.want, report.Checks)
			}
		})
	}
}

func TestPreparationDefersOnlyStorageFloor(t *testing.T) {
	facts := Facts{OS: "linux", Architecture: "amd64", Distribution: "ubuntu", DistributionVersion: "24.04", KVMUsable: true, AppArmorEnforcing: true, LibvirtReachable: true, MemoryBytes: 16 * gib, AvailableDiskBytes: 8 * gib, DiskProbePath: "/var/lib"}
	report := Evaluate(facts, time.Now())
	if report.PrerequisitesMet || !report.PreparationPrerequisitesMet() {
		t.Fatal("storage floor not deferred")
	}
	facts.AvailableDiskBytes = 0
	if Evaluate(facts, time.Now()).PreparationPrerequisitesMet() {
		t.Fatal("missing disk observation admitted")
	}
	facts.AvailableDiskBytes = 8 * gib
	facts.KVMUsable = false
	if Evaluate(facts, time.Now()).PreparationPrerequisitesMet() {
		t.Fatal("non-storage failure bypassed")
	}
}
