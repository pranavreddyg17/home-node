//go:build linux

package backup

import (
	"context"
	"github.com/pranavreddyg17/home-node/internal/disktransport"
	"github.com/pranavreddyg17/home-node/internal/state"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLaunchCompletionRequiresOwnDomainAndJob(t *testing.T) {
	launch := Launch{Version: 2, JobID: state.Random(), DeviceID: state.Random(), ManagementToken: state.Random(), Release: "0.1.0", CatalogVersion: 1}
	for _, scenario := range []string{"valid", "foreign-job", "dispatch-domain"} {
		t.Run(scenario, func(t *testing.T) {
			sender, receiver := dispatchPair(t)
			raw := launchCompletionPacket(launch)
			if scenario == "foreign-job" {
				other := launch
				other.JobID = state.Random()
				raw = launchCompletionPacket(other)
			}
			if scenario == "dispatch-domain" {
				dispatch, _ := launch.AcquiredDispatch(state.Random())
				raw = completionPacket(dispatch)
			}
			if err := disktransport.SendPacket(context.Background(), receiver, raw); err != nil {
				t.Fatal(err)
			}
			if err := waitLaunchCompletion(context.Background(), sender, launch); (err == nil) != (scenario == "valid") {
				t.Fatal("unexpected completion qualification", err)
			}
		})
	}
}

func TestAcknowledgedLaunchServerClosesCredentialBeforeReply(t *testing.T) {
	uid := uint32(os.Geteuid())
	if uid == 0 {
		t.Skip("private peers must be unprivileged")
	}
	path := filepath.Join(t.TempDir(), "launch.sock")
	listener, err := net.ListenUnix("unixpacket", &net.UnixAddr{Net: "unixpacket", Name: path})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	launch := Launch{Version: 2, JobID: state.Random(), DeviceID: state.Random(), ManagementToken: state.Random(), Release: "0.1.0", CatalogVersion: 1}
	received := make(chan *os.File, 1)
	done := make(chan error, 1)
	go func() {
		done <- ServeAcknowledgedCredentialLaunch(ctx, listener, uid, func(_ context.Context, got Launch, file *os.File) error {
			if got != launch || file == nil {
				return ErrManifest
			}
			received <- file
			return nil
		})
	}()
	connection, err := net.DialUnix("unixpacket", nil, &net.UnixAddr{Net: "unixpacket", Name: path})
	if err != nil {
		t.Fatal(err)
	}
	defer connection.Close()
	source, err := CreateRepositoryPassword([]byte("fixture-launch-secret"))
	if err != nil {
		t.Fatal(err)
	}
	defer source.Close()
	raw, _ := EncodeLaunch(launch)
	if err = sendCredentialPayload(ctx, connection, uid, raw, source); err != nil {
		t.Fatal(err)
	}
	bounded, stop := context.WithTimeout(ctx, time.Second)
	defer stop()
	if err = waitLaunchCompletion(bounded, connection, launch); err != nil {
		t.Fatal(err)
	}
	file := <-received
	if _, err = file.Stat(); err == nil {
		t.Fatal("completion preceded credential closure")
	}
	if _, err = source.Stat(); err != nil {
		t.Fatal("server closed source credential", err)
	}
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("launch server did not drain")
	}
}
