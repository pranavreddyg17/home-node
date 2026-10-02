package updates

import (
	"strings"
	"testing"
)

func TestInspectionResourcesRefuseRelaxedManagerLimits(t *testing.T) {
	properties := "MemoryMax=268435456\nMemorySwapMax=0\nCPUQuotaPerSecUSec=500ms\nTasksMax=32\nOOMPolicy=kill\nKillMode=control-group\nRestart=no\nTimeoutStartUSec=2min 30s\nTimeoutStopUSec=5s\n"
	if err := ValidateInspectionResources([]byte(properties)); err != nil {
		t.Fatal(err)
	}
	for _, pair := range [][2]string{{"MemoryMax=268435456", "MemoryMax=infinity"}, {"MemorySwapMax=0", "MemorySwapMax=268435456"}, {"CPUQuotaPerSecUSec=500ms", "CPUQuotaPerSecUSec=infinity"}, {"TasksMax=32", "TasksMax=512"}, {"OOMPolicy=kill", "OOMPolicy=continue"}, {"KillMode=control-group", "KillMode=process"}, {"Restart=no", "Restart=on-failure"}, {"TimeoutStartUSec=2min 30s", "TimeoutStartUSec=infinity"}, {"TimeoutStopUSec=5s", "TimeoutStopUSec=infinity"}} {
		if err := ValidateInspectionResources([]byte(strings.Replace(properties, pair[0], pair[1], 1))); err == nil {
			t.Fatal("relaxed limit accepted", pair)
		}
	}
	for _, data := range []string{properties + "TasksMax=32\n", properties + "Unknown=1\n", strings.Replace(properties, "MemorySwapMax=0\n", "", 1), strings.Replace(properties, "Restart=no", "Restart=no\r", 1), strings.Repeat("x", 2049)} {
		if err := ValidateInspectionResources([]byte(data)); err == nil {
			t.Fatal("ambiguous resource snapshot accepted")
		}
	}
}
