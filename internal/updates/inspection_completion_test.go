package updates

import (
	"strings"
	"testing"
)

func TestInspectionCompletionRequiresFreshSuccessfulInvocation(t *testing.T) {
	invocation := strings.Repeat("a", 32)
	properties := "InvocationID=" + invocation + "\nResult=success\nExecMainCode=1\nExecMainStatus=0\nActiveState=inactive\nSubState=dead\nExecMainStartTimestampMonotonic=200\nExecMainExitTimestampMonotonic=300\n"
	if err := ValidateInspectionCompletion([]byte(properties), invocation, 100); err != nil {
		t.Fatal(err)
	}
	for _, pair := range [][2]string{{invocation, strings.Repeat("b", 32)}, {"Result=success", "Result=timeout"}, {"ExecMainCode=1", "ExecMainCode=2"}, {"ExecMainStatus=0", "ExecMainStatus=1"}, {"ActiveState=inactive", "ActiveState=active"}, {"SubState=dead", "SubState=running"}, {"Monotonic=200", "Monotonic=99"}, {"Monotonic=300", "Monotonic=199"}, {"Monotonic=200", "Monotonic=0200"}, {"Monotonic=300", "Monotonic=+300"}} {
		changed := strings.Replace(properties, pair[0], pair[1], 1)
		if err := ValidateInspectionCompletion([]byte(changed), invocation, 100); err == nil {
			t.Fatal("unsafe completion accepted", pair)
		}
	}
	for _, changed := range []string{properties + "Result=success\n", properties + "Unknown=1\n", strings.Replace(properties, "Result=success\n", "", 1), strings.Replace(properties, "success", "success\r", 1), strings.Repeat("x", 2049)} {
		if err := ValidateInspectionCompletion([]byte(changed), invocation, 100); err == nil {
			t.Fatal("ambiguous completion accepted")
		}
	}
	if err := ValidateInspectionCompletion([]byte(properties), invocation, 201); err == nil {
		t.Fatal("stale completion accepted")
	}
	if err := ValidateInspectionCompletion([]byte(properties), invocation, 0); err == nil {
		t.Fatal("missing start boundary accepted")
	}
}

func TestInspectionInvocationHasFixedCanonicalBound(t *testing.T) {
	for _, value := range []string{"", strings.Repeat("0", 32), strings.Repeat("A", 32), strings.Repeat("g", 32), strings.Repeat("a", 31), strings.Repeat("a", 33), strings.Repeat("a", 1<<20), strings.Repeat("a", 31) + "\n"} {
		if validInspectionInvocation(value) {
			t.Fatal("noncanonical invocation accepted")
		}
	}
	for _, value := range []string{strings.Repeat("a", 32), strings.Repeat("0", 31) + "1", "0123456789abcdef0123456789abcdef"} {
		if !validInspectionInvocation(value) {
			t.Fatal("canonical invocation refused", value)
		}
	}
	oversized := strings.Repeat("a", 1<<20)
	if allocations := testing.AllocsPerRun(100, func() { validInspectionInvocation(oversized) }); allocations != 0 {
		t.Fatal("oversized identity allocated", allocations)
	}
}

func TestInspectionInvocationCaptureRequiresFreshOneshot(t *testing.T) {
	invocation := strings.Repeat("a", 32)
	active := "InvocationID=" + invocation + "\nResult=success\nExecMainCode=0\nExecMainStatus=0\nActiveState=activating\nSubState=start\nExecMainStartTimestampMonotonic=200\nExecMainExitTimestampMonotonic=0\n"
	if captured, err := InspectionInvocationFromManager([]byte(active), 100); err != nil || captured != invocation {
		t.Fatal("fresh running invocation refused", captured, err)
	}
	for _, pair := range [][2]string{{"Monotonic=200", "Monotonic=99"}, {"Result=success", "Result=exit-code"}, {"SubState=start", "SubState=start-pre"}, {"ExecMainCode=0", "ExecMainCode=1"}, {"ExecMainStatus=0", "ExecMainStatus=1"}, {"ExitTimestampMonotonic=0", "ExitTimestampMonotonic=300"}, {"ActiveState=activating", "ActiveState=active"}} {
		if _, err := InspectionInvocationFromManager([]byte(strings.Replace(active, pair[0], pair[1], 1)), 100); err == nil {
			t.Fatal("unsafe invocation capture accepted", pair)
		}
	}
	completed := strings.ReplaceAll(active, "ExecMainCode=0", "ExecMainCode=1")
	completed = strings.ReplaceAll(completed, "ActiveState=activating", "ActiveState=inactive")
	completed = strings.ReplaceAll(completed, "SubState=start", "SubState=dead")
	completed = strings.ReplaceAll(completed, "ExitTimestampMonotonic=0", "ExitTimestampMonotonic=300")
	if captured, err := InspectionInvocationFromManager([]byte(completed), 100); err != nil || captured != invocation {
		t.Fatal("fast completed invocation refused", captured, err)
	}
	if _, err := InspectionInvocationFromManager([]byte(active), 0); err == nil {
		t.Fatal("missing start boundary accepted")
	}
}
