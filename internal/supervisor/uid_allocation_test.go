package supervisor

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestGuestUIDAutomaticAllocationExclusions(t *testing.T) {
	config := "UID_MIN 1000\nUID_MAX 60000\nSYS_UID_MIN 100\nSYS_UID_MAX 999\nSUB_UID_MIN 100000\nSUB_UID_MAX 600100000\n"
	pool := GuestUIDPool{First: 2000000000, Last: 2000000001}
	if err := validateGuestUIDAutomaticAllocation(context.Background(), pool, []byte(config)); err != nil {
		t.Fatal(err)
	}
	for _, candidate := range []GuestUIDPool{{First: 200000, Last: 200001}, {First: 1000000000, Last: 1000000001}, {First: 1879048191, Last: 1879048192}} {
		if err := validateGuestUIDAutomaticAllocation(context.Background(), candidate, []byte(config)); err == nil {
			t.Fatal("automatic allocation overlap admitted", candidate)
		}
	}
	for _, bad := range []string{
		config + "UID_MAX 60000\n",
		strings.Replace(config, "UID_MAX 60000", "UID_MAX +60000", 1),
		strings.Replace(config, "UID_MAX 60000", "UID_MAX 4294967296", 1),
		strings.Replace(config, "SYS_UID_MIN 100\n", "", 1),
		strings.Replace(config, "SYS_UID_MAX 999", "SYS_UID_MAX 99", 1),
		strings.Replace(config, "SUB_UID_MAX 600100000", "SUB_UID_MAX 2000000000", 1),
		strings.Replace(config, "UID_MAX 60000", "UID_MAX 2000000001 extra", 1),
		config + "\x00",
	} {
		if err := validateGuestUIDAutomaticAllocation(context.Background(), pool, []byte(bad)); err == nil {
			t.Fatal("ambiguous allocation config admitted", bad)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := validateGuestUIDAutomaticAllocation(ctx, pool, []byte(config)); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
}

func TestGuestUIDAllocationDiagnosticsIdentifyPolicyWithoutHostValues(t *testing.T) {
	base := "UID_MIN 1000\nUID_MAX 60000\nSYS_UID_MIN 100\nSYS_UID_MAX 999\nSUB_UID_MIN 100000\nSUB_UID_MAX 600100000\n"
	pool := GuestUIDPool{First: 2000000000, Last: 2000000001}
	for _, test := range []struct{ data, want, hidden string }{
		{strings.Replace(base, "SYS_UID_MIN 100\n", "", 1), "explicit SYS_UID_MIN", "600100000"},
		{base + "UID_MIN 314159265\n", "one explicit UID_MIN", "314159265"},
		{strings.Replace(base, "SYS_UID_MAX 999", "SYS_UID_MAX 42", 1), "ordered nonzero SYS_UID range", "42"},
		{strings.Replace(base, "SUB_UID_MAX 600100000", "SUB_UID_MAX 2000000000", 1), "overlaps the SUB_UID", "2000000000"},
		{strings.Replace(base, "UID_MAX 60000", "UID_MAX secret-invalid-value", 1), "decimal UID_MAX", "secret-invalid-value"},
	} {
		err := validateGuestUIDAutomaticAllocation(context.Background(), pool, []byte(test.data))
		if !errors.Is(err, ErrPolicy) || !strings.Contains(err.Error(), test.want) || strings.Contains(err.Error(), test.hidden) {
			t.Fatal("allocation diagnostic lost policy context or disclosed configuration", err)
		}
	}
}
