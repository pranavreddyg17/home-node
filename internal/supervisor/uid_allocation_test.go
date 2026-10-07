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
