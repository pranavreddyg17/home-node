package supervisor

import (
	"context"
	"errors"
	"testing"

	"github.com/pranavreddyg17/home-node/internal/state"
)

func TestDomainIdentityBindingPersistsAndRefusesDowngrade(t *testing.T) {
	m, _ := newManager(t)
	ctx := context.Background()
	pool := GuestUIDPool{First: 200000, Last: 200001}
	m.GuestUIDPool, m.GuestGID = &pool, 64055
	id := state.Random()
	absent := Domain{ID: id}
	if err := m.bindDomainGuestIdentity(ctx, &absent, false); err == nil {
		t.Fatal("audit allocated absent identity")
	}
	d := Domain{ID: id}
	if err := m.bindDomainGuestIdentity(ctx, &d, true); err != nil {
		t.Fatal(err)
	}
	if d.GuestUID != pool.First || d.GuestGID != 64055 {
		t.Fatal(d)
	}
	for _, reserve := range []bool{false, true} {
		retry := Domain{ID: id}
		if err := m.bindDomainGuestIdentity(ctx, &retry, reserve); err != nil || retry.GuestUID != d.GuestUID || retry.GuestGID != d.GuestGID {
			t.Fatal("identity drift", retry, err)
		}
	}
	m.GuestGID++
	if err := m.bindDomainGuestIdentity(ctx, &Domain{ID: id}, false); !errors.Is(err, ErrPolicy) {
		t.Fatal("changed group accepted", err)
	}
	m.GuestGID = 64055
	pool.Last++
	if err := m.bindDomainGuestIdentity(ctx, &Domain{ID: id}, false); !errors.Is(err, ErrPolicy) {
		t.Fatal("changed pool accepted", err)
	}
	pool.Last--
	pool.Blocked = map[uint32]bool{d.GuestUID: true}
	if err := m.bindDomainGuestIdentity(ctx, &Domain{ID: id}, false); !errors.Is(err, ErrPolicy) {
		t.Fatal("conflicting UID accepted", err)
	}
	m.GuestUIDPool, m.GuestGID = nil, 0
	if err := m.bindDomainGuestIdentity(ctx, &Domain{ID: id}, false); !errors.Is(err, ErrPolicy) {
		t.Fatal("identity downgraded", err)
	}
	ctx, cancel := context.WithCancel(ctx)
	cancel()
	if err := m.bindDomainGuestIdentity(ctx, &Domain{ID: id}, false); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
}

type identityBackend struct {
	*fakeBackend
	prepared, started, verified []Domain
}

func (b *identityBackend) Prepare(_ context.Context, d Domain) error {
	b.prepared = append(b.prepared, d)
	return nil
}
func (b *identityBackend) Start(ctx context.Context, d Domain) error {
	b.started = append(b.started, d)
	return b.fakeBackend.Start(ctx, d)
}
func (b *identityBackend) Verify(_ context.Context, d Domain) error {
	b.verified = append(b.verified, d)
	return nil
}

func TestManagerLaunchAndAuditUseSameGuestIdentity(t *testing.T) {
	m, b := newManager(t)
	pool := GuestUIDPool{First: 200000, Last: 200001}
	m.GuestUIDPool, m.GuestGID = &pool, 64055
	backend := &identityBackend{fakeBackend: b}
	m.Backend = backend
	if _, err := m.Apply(context.Background(), startRequest()); err != nil {
		t.Fatal(err)
	}
	if err := m.Audit(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(backend.prepared) != 1 || len(backend.started) != 1 || len(backend.verified) != 2 {
		t.Fatal("missing lifecycle calls", backend)
	}
	for _, d := range append(append(backend.prepared, backend.started...), backend.verified...) {
		if d.GuestUID != pool.First || d.GuestGID != 64055 {
			t.Fatal("lifecycle identity omitted", d)
		}
	}
	m.GuestUIDPool, m.GuestGID = nil, 0
	if err := m.Audit(context.Background()); err != nil {
		t.Fatal(err)
	}
	if b.stops != 1 || len(backend.verified) != 2 {
		t.Fatal("policy removal did not stop before verification", b.stops, len(backend.verified))
	}
}

func TestManagerInitializationRefusesGuestIdentityPolicyDrift(t *testing.T) {
	m, _ := newManager(t)
	pool := GuestUIDPool{First: 200000, Last: 200001}
	m.GuestUIDPool, m.GuestGID = &pool, 64055
	d := Domain{ID: state.Random()}
	if err := m.bindDomainGuestIdentity(context.Background(), &d, true); err != nil {
		t.Fatal(err)
	}
	if err := m.Initialize(context.Background()); err != nil {
		t.Fatal("matching startup policy refused", err)
	}
	m.GuestGID++
	if err := m.Initialize(context.Background()); !errors.Is(err, ErrPolicy) {
		t.Fatal("group drift accepted at startup", err)
	}
	m.GuestGID = 64055
	pool.Last++
	if err := m.Initialize(context.Background()); !errors.Is(err, ErrPolicy) {
		t.Fatal("pool drift accepted at startup", err)
	}
	pool.Last--
	m.GuestUIDPool, m.GuestGID = nil, 0
	if err := m.Initialize(context.Background()); !errors.Is(err, ErrPolicy) {
		t.Fatal("shared identity downgrade accepted at startup", err)
	}
	m.GuestUIDPool, m.GuestGID = &pool, 64055
	if err := m.Initialize(context.Background()); err != nil {
		t.Fatal("refusal damaged original policy", err)
	}
	if _, err := m.Store.DB.Exec("INSERT INTO runtime_guest_groups VALUES(?,?)", state.Random(), 64055); err != nil {
		t.Fatal(err)
	}
	if err := m.Initialize(context.Background()); !errors.Is(err, ErrPolicy) {
		t.Fatal("orphan group accepted", err)
	}
}
