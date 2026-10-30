package supervisor

import (
	"errors"
	"strings"
	"testing"
)

func TestQEMUSeccompStatus(t *testing.T) {
	for _, valid := range []string{"Name:\tqemu\nSeccomp:\t2\nSeccomp_filters:\t1\n", "Seccomp_filters: 3\nSeccomp: 2\n"} {
		if err := validateQEMUSeccompStatus([]byte(valid)); err != nil {
			t.Fatal(err)
		}
	}
	for _, invalid := range []string{"", "Seccomp: 2\n", "Seccomp_filters: 1\n", "Seccomp: 0\nSeccomp_filters: 1\n", "Seccomp: 1\nSeccomp_filters: 1\n", "Seccomp: 02\nSeccomp_filters: 1\n", "Seccomp: 2\nSeccomp_filters: 0\n", "Seccomp: 2\nSeccomp_filters: -1\n", "Seccomp: 2\nSeccomp_filters: +1\n", "Seccomp: 2\nSeccomp_filters: 01\n", "Seccomp: 2\nSeccomp_filters: 4294967296\n", "Seccomp: 2\nSeccomp_filters: 1 2\n", "Seccomp: 2\nSeccomp: 2\nSeccomp_filters: 1\n", "Seccomp: 2\nSeccomp_filters: 1\nSeccomp_filters: 1\n", "Seccomp: 2\nSeccomp_filters: 1\r\n", "Seccomp: 2\nSeccomp_filters: 1\x00\n", strings.Repeat("x", (64<<10)+1), string([]byte{255})} {
		if err := validateQEMUSeccompStatus([]byte(invalid)); !errors.Is(err, ErrPolicy) {
			t.Fatal("unqualified seccomp status admitted", err)
		}
	}
}
