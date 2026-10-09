package supervisor

import (
	"context"
	"errors"
	"testing"
)

type changingAuditBackend struct {
	*fakeBackend
	change func() error
}

func (b *changingAuditBackend) Verify(context.Context, Domain) error { return b.change() }

func TestAuditFailureCannotStopNewRuntimeRevision(t *testing.T) {
	m, backend := newManager(t)
	ctx := context.Background()
	r := startRequest()
	if _, err := m.Apply(ctx, r); err != nil {
		t.Fatal(err)
	}
	m.Backend = &changingAuditBackend{fakeBackend: backend, change: func() error {
		if _, err := m.Store.DB.Exec(`UPDATE runtime_instances SET revision=2,state='preparing' WHERE id=?`, r.InstanceID); err != nil {
			return err
		}
		return errors.New("old guest confinement failed")
	}}
	if err := m.Audit(ctx); err != nil {
		t.Fatal(err)
	}
	if backend.stops != 0 {
		t.Fatal("stale audit stopped new preparation", backend.stops)
	}
	i, err := m.Inspect(ctx, r.InstanceID)
	if err != nil || i.Revision != 2 || i.State != "preparing" || i.Desired != "running" {
		t.Fatal("new revision overwritten", i, err)
	}
}
