package supervisor

import (
	"strings"

	"github.com/pranavreddyg17/home-node/internal/guestproto"
)

// runtimeUUID must come from the authenticated local libvirt connection for the
// exact protected domain name. This binds a label, not loaded policy contents.
func validateDomainAppArmorIdentity(d Domain, runtimeUUID, label string) error {
	if !guestproto.ValidID(d.ID) || len(runtimeUUID) != 36 {
		return ErrPolicy
	}
	for i, ch := range runtimeUUID {
		if i == 8 || i == 13 || i == 18 || i == 23 {
			if ch != '-' {
				return ErrPolicy
			}
		} else if !strings.ContainsRune("0123456789abcdef", ch) {
			return ErrPolicy
		}
	}
	if d.ConfinedWrites && runtimeUUID != d.RuntimeUUID() {
		return ErrPolicy
	}
	if strings.TrimSpace(label) != "libvirt-"+runtimeUUID+" (enforce)" {
		return ErrPolicy
	}
	return nil
}
