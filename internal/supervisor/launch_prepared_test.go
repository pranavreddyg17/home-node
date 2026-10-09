package supervisor

import (
	"context"
	"errors"
	"testing"
)

type revokedLaunchBackend struct {
	*fakeBackend
	revoke func()
}

func (b *revokedLaunchBackend) Start(ctx context.Context, d Domain) error {
	err := b.fakeBackend.Start(ctx, d)
	b.revoke()
	return err
}

func TestLaunchCannotPublishAfterAuthorityRevocation(t *testing.T) {
	m, backend := newManager(t)
	ctx := context.Background()
	r := startRequest()
	if _, err := m.Apply(ctx, r); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Store.DB.Exec("UPDATE runtime_instances SET state='preparing' WHERE id=?", r.InstanceID); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Store.DB.Exec("UPDATE runtime_operations SET state='pending' WHERE id=?", r.OperationID); err != nil {
		t.Fatal(err)
	}
	backend.running = false
	revoked := false
	m.Backend = &revokedLaunchBackend{fakeBackend: backend, revoke: func() { revoked = true }}
	cause := errors.New("retained preparation authority revoked")
	err := m.launchPreparedDomain(ctx, r, Domain{ID: r.InstanceID}, func(context.Context) error {
		if revoked {
			return cause
		}
		return nil
	})
	if !errors.Is(err, cause) {
		t.Fatal("revoked launch published", err)
	}
	i, err := m.Inspect(ctx, r.InstanceID)
	if err != nil || i.State != "preparing" {
		t.Fatal("running publication crossed revocation", i, err)
	}
	var phase string
	if err := m.Store.DB.QueryRow("SELECT state FROM runtime_operations WHERE id=?", r.OperationID).Scan(&phase); err != nil || phase != "pending" {
		t.Fatal("operation published after revocation", phase, err)
	}
}
