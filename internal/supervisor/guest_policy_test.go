package supervisor

import (
	"errors"
	"testing"
)

func TestReservedGuestPolicyRequiresEntireIndependentRange(t *testing.T) {
	valid := ReservedGuestPolicy{Version: 1, FirstUID: 200000, LastUID: 200002, GuestGID: 994}
	if err := valid.Validate(1001, 1002); err != nil {
		t.Fatal(err)
	}
	for _, change := range []func(*ReservedGuestPolicy){
		func(p *ReservedGuestPolicy) { p.Version = 2 },
		func(p *ReservedGuestPolicy) { p.FirstUID = 0 },
		func(p *ReservedGuestPolicy) { p.LastUID = p.FirstUID - 1 },
		func(p *ReservedGuestPolicy) { p.LastUID = p.FirstUID + 65536 },
		func(p *ReservedGuestPolicy) { p.GuestGID = 0 },
	} {
		candidate := valid
		change(&candidate)
		if err := candidate.Validate(1001, 1002); !errors.Is(err, ErrPolicy) {
			t.Fatal(candidate, err)
		}
	}
	for _, uid := range []uint32{valid.FirstUID, valid.LastUID} {
		if err := valid.Validate(uid); !errors.Is(err, ErrPolicy) {
			t.Fatal("service range collision", uid, err)
		}
	}
}
