package supervisor

import (
	"errors"
	"strings"
	"testing"
)

func TestGuestDACCredentialsRequireExactIdentityAndNoPrivilege(t *testing.T) {
	valid := "Name:\tqemu\nUid:\t200000\t200000\t200000\t200000\nGid:\t64055\t64055\t64055\t64055\nGroups:\t64055\nCapInh:\t0000000000000000\nCapPrm:\t0000000000000000\nCapEff:\t0000000000000000\nCapAmb:\t0000000000000000\nNoNewPrivs:\t1\n"
	if err := validateGuestDACCredentials([]byte(valid), 200000, 64055); err != nil {
		t.Fatal(err)
	}
	for _, data := range []string{
		strings.Replace(valid, "Uid:\t200000", "Uid:\t0", 1),
		strings.Replace(valid, "Gid:\t64055", "Gid:\t0", 1),
		strings.Replace(valid, "Groups:\t64055", "Groups:\t0", 1),
		strings.Replace(valid, "Groups:\t64055", "Groups:\t64055 108", 1),
		strings.Replace(valid, "CapEff:\t0000000000000000", "CapEff:\t0000000000000001", 1),
		strings.Replace(valid, "CapAmb:\t0000000000000000\n", "", 1),
		strings.Replace(valid, "NoNewPrivs:\t1", "NoNewPrivs:\t0", 1),
		valid + "Uid:\t200000\t200000\t200000\t200000\n",
	} {
		if err := validateGuestDACCredentials([]byte(data), 200000, 64055); !errors.Is(err, ErrPolicy) {
			t.Fatal("privileged/foreign credentials accepted", err)
		}
	}
}
