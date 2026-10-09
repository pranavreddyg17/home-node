package supervisor

import (
	"context"
	"errors"
	"testing"
)

func TestPreparedDomainVerificationDispatch(t *testing.T) {
	ctx := context.Background()
	m, backend := newManager(t)
	sentinel := errors.New("confinement failed")
	backend.verifyErr = sentinel
	if err := m.verifyPreparedDomain(ctx, Domain{}, 1); !errors.Is(err, sentinel) {
		t.Fatal("shared domain confinement failure lost", err)
	}
	if err := m.verifyPreparedDomain(ctx, Domain{GuestUID: 200000}, 1); !errors.Is(err, sentinel) {
		t.Fatal("injected backend confinement failure lost", err)
	}
	m.Backend = LinuxBackend{}
	if err := m.verifyPreparedDomain(ctx, Domain{GuestUID: 200000}, 1); !errors.Is(err, ErrPolicy) {
		t.Fatal("unconfigured Linux backend admitted reserved socket", err)
	}
	ctx, cancel := context.WithCancel(ctx)
	cancel()
	if err := m.verifyPreparedDomain(ctx, Domain{}, 1); !errors.Is(err, context.Canceled) {
		t.Fatal("cancellation lost", err)
	}
}
