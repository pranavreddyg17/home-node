package backup

import (
	"bytes"
	"context"
	"errors"
	"net"
	"time"

	"github.com/pranavreddyg17/home-node/internal/disktransport"
)

func launchRepositoryRefusalPacket(launch Launch) []byte {
	return []byte("homenode-backup-launch-repository-refused-v2 " + launch.JobID)
}

// Caller must have joined the callback and closed its received credential before
// sending. No arbitrary failure can be upgraded to a pre-acquisition refusal.
func sendLaunchRepositoryRefusal(ctx context.Context, connection *net.UnixConn, launch Launch, cause error) error {
	if !launch.valid() || !errors.Is(cause, ErrLaunchRepositoryAdmission) {
		return ErrManifest
	}
	bounded, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	return disktransport.SendPacket(bounded, connection, launchRepositoryRefusalPacket(launch))
}

func parseLaunchCompletionPacket(raw []byte, launch Launch) error {
	if !launch.valid() {
		return ErrManifest
	}
	if bytes.Equal(raw, launchCompletionPacket(launch)) {
		return nil
	}
	if bytes.Equal(raw, launchRepositoryRefusalPacket(launch)) {
		return ErrLaunchRepositoryRefusedStopped
	}
	return ErrManifest
}
