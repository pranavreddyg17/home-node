package install

import (
	"errors"
	"testing"
)

func TestRecoveryEmptyCgroupRefusesPopulatedAndAmbiguousState(t *testing.T) {
	if err := validateRecoveryEmptyCgroup([]byte("populated 0\nfrozen 0\n")); err != nil {
		t.Fatal(err)
	}
	for _, data := range []string{"populated 1\nfrozen 0\n", "populated 0\nfrozen 1\n", "populated 0\n", "populated 0\nfrozen 0\npopulated 0\n", "populated 0\nfrozen 0\nunknown 0\n", "populated  0\nfrozen 0\n", "populated 0\r\nfrozen 0\r\n"} {
		if err := validateRecoveryEmptyCgroup([]byte(data)); !errors.Is(err, ErrConflict) {
			t.Fatal("unsafe cgroup state admitted", data, err)
		}
	}
}
