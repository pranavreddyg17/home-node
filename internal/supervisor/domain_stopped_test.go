package supervisor

import (
	"context"
	"errors"
	"testing"
)

func TestReservedDomainStoppedGuardRequiresQualifiedBackend(t *testing.T) {
	m, backend := newManager(t)
	pool := GuestUIDPool{First: 200000, Last: 200002}
	m.GuestUIDPool, m.GuestGID = &pool, 64055
	if err := m.checkReservedDomainStopped(context.Background(), Domain{GuestUID: 200000, GuestGID: 64055}); !errors.Is(err, ErrPolicy) {
		t.Fatal("unqualified backend supplied runtime exclusion", err)
	}
	if backend.starts != 0 {
		t.Fatal("stopped observation activated a guest")
	}
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	if err := m.checkReservedDomainStopped(canceled, Domain{}); !errors.Is(err, context.Canceled) {
		t.Fatal("canceled stopped observation", err)
	}
}
