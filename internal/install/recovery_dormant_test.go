package install

import (
	"errors"
	"strings"
	"testing"
)

func TestRecoveryDormancyRequiresExactInactiveProcessState(t *testing.T) {
	valid := "LoadState=loaded\nActiveState=inactive\nSubState=dead\nMainPID=0\nControlPID=0\n"
	if err := validateRecoveryDormant([]byte(valid)); err != nil {
		t.Fatal(err)
	}
	for _, data := range []string{
		strings.Replace(valid, "inactive", "active", 1),
		strings.Replace(valid, "dead", "stop", 1),
		strings.Replace(valid, "MainPID=0", "MainPID=123", 1),
		strings.Replace(valid, "ControlPID=0", "ControlPID=123", 1),
		strings.Replace(valid, "loaded", "not-found", 1),
		strings.Replace(valid, "MainPID=0\n", "", 1),
		valid + "ActiveState=inactive\n", valid + "Unknown=0\n",
		strings.ReplaceAll(valid, "\n", "\r\n"), strings.Repeat(valid, 100),
	} {
		if err := validateRecoveryDormant([]byte(data)); !errors.Is(err, ErrConflict) {
			t.Fatal("unsafe service state admitted", data, err)
		}
	}
}
