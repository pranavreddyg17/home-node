package updates

import (
	"strings"
	"testing"
)

func TestInspectionIsolationRefusesRelaxedConfinement(t *testing.T) {
	properties := "NoNewPrivileges=yes\nCapabilityBoundingSet=\nAmbientCapabilities=\nProtectSystem=strict\nProtectHome=yes\nPrivateTmp=yes\nPrivateDevices=yes\nPrivateNetwork=yes\nProtectKernelTunables=yes\nProtectKernelModules=yes\nProtectKernelLogs=yes\nProtectControlGroups=yes\nProtectProc=invisible\nProcSubset=pid\nRestrictSUIDSGID=yes\nRestrictRealtime=yes\nLockPersonality=yes\nUMask=0077\nSupplementaryGroups=\n"
	if err := ValidateInspectionIsolation([]byte(properties)); err != nil {
		t.Fatal(err)
	}
	for _, line := range strings.Split(strings.TrimSuffix(properties, "\n"), "\n") {
		name, _, _ := strings.Cut(line, "=")
		changed := strings.Replace(properties, line, name+"=unsafe", 1)
		if err := ValidateInspectionIsolation([]byte(changed)); err == nil {
			t.Fatal("relaxed confinement accepted", name)
		}
		missing := strings.Replace(properties, line+"\n", "", 1)
		if err := ValidateInspectionIsolation([]byte(missing)); err == nil {
			t.Fatal("missing confinement accepted", name)
		}
	}
	for _, data := range []string{properties + "NoNewPrivileges=yes\n", properties + "Unknown=yes\n", strings.Replace(properties, "strict", "strict\r", 1), strings.Repeat("x", 2049)} {
		if err := ValidateInspectionIsolation([]byte(data)); err == nil {
			t.Fatal("ambiguous confinement snapshot accepted")
		}
	}
}
