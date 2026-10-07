package supervisor

// UnixPeerIdentity is the kernel credential snapshot associated with a Unix
// connection. It does not establish current process identity after PID reuse.
type UnixPeerIdentity struct {
	PID      int32
	UID, GID uint32
}
