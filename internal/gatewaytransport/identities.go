package gatewaytransport

// ValidateRoleGroups keeps the TLS-key group separate from every controller
// and runtime permission group, including under accidentally aliased NSS names.
func ValidateRoleGroups(controller, gateway, proxy, runtime uint32) error {
	seen := make(map[uint32]bool, 4)
	for _, gid := range []uint32{controller, gateway, proxy, runtime} {
		if gid == 0 || gid > 1<<31-1 || seen[gid] {
			return ErrTransport
		}
		seen[gid] = true
	}
	return nil
}
