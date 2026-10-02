package backup

import (
	"bytes"
	"context"
	"net"
	"os"
	"time"

	"github.com/pranavreddyg17/home-node/internal/disktransport"
)

func completionPacket(dispatch Dispatch) []byte {
	return []byte("homenode-backup-complete-v1 " + dispatch.JobID)
}

func sendWorkerCompletion(ctx context.Context, connection *net.UnixConn, dispatch Dispatch) error {
	if !dispatch.valid() {
		return ErrManifest
	}
	bounded, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	return disktransport.SendPacket(bounded, connection, completionPacket(dispatch))
}

// SendActivatedCredentialDispatchAndWait uses only the trusted root-created
// activated listener. It never retries. A matching response proves successful
// worker callback completion, not repository health or publication persistence.
// On any delivery/response error the coordinator must reconcile its owned job.
func SendActivatedCredentialDispatchAndWait(ctx context.Context, connection *net.UnixConn, dispatch Dispatch, credential *os.File) error {
	if err := SendActivatedCredentialDispatch(ctx, connection, dispatch, credential); err != nil {
		return err
	}
	bounded, cancel := context.WithTimeout(ctx, 2*time.Hour)
	defer cancel()
	raw, err := disktransport.ReceivePacket(bounded, connection)
	if err != nil {
		return err
	}
	if !bytes.Equal(raw, completionPacket(dispatch)) {
		return ErrManifest
	}
	return bounded.Err()
}

func launchCompletionPacket(launch Launch) []byte {
	return []byte("homenode-backup-launch-complete-v2 " + launch.JobID)
}
func sendLaunchCompletion(ctx context.Context, connection *net.UnixConn, launch Launch) error {
	if !launch.valid() {
		return ErrManifest
	}
	bounded, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	return disktransport.SendPacket(bounded, connection, launchCompletionPacket(launch))
}

// SendActivatedCredentialLaunchAndWait never retries after delivery. The reply
// must match the preliminary launch domain and job; publication remains subject
// to independently inspected durable outcomes and recovery barriers.
func SendActivatedCredentialLaunchAndWait(ctx context.Context, connection *net.UnixConn, launch Launch, credential *os.File) error {
	if err := SendActivatedCredentialLaunch(ctx, connection, launch, credential); err != nil {
		return err
	}
	return waitLaunchCompletion(ctx, connection, launch)
}
func waitLaunchCompletion(ctx context.Context, connection *net.UnixConn, launch Launch) error {
	bounded, cancel := context.WithTimeout(ctx, 2*time.Hour)
	defer cancel()
	raw, err := disktransport.ReceivePacket(bounded, connection)
	if err != nil {
		return err
	}
	if !bytes.Equal(raw, launchCompletionPacket(launch)) {
		return ErrManifest
	}
	return bounded.Err()
}
