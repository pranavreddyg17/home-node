//go:build linux

package backup

import (
	"bytes"
	"context"
	"net"
	"os"
	"testing"
	"time"

	"github.com/pranavreddyg17/home-node/internal/disktransport"
	"github.com/pranavreddyg17/home-node/internal/state"
)

func TestCredentialDispatchTransfersSealedReadonlyDuplicate(t *testing.T) {
	uid := uint32(os.Geteuid())
	if uid == 0 {
		t.Skip("private peers must be unprivileged")
	}
	sender, receiver := dispatchPair(t)
	job := Dispatch{Version: 1, JobID: state.Random(), DeviceID: state.Random(), ManagementToken: state.Random(), RuntimeToken: state.Random(), Release: "0.1.0", CatalogVersion: 1}
	secret := []byte("fixture-only-credential")
	source, err := CreateRepositoryPassword(secret)
	if err != nil {
		t.Fatal(err)
	}
	defer source.Close()
	if err = SendCredentialDispatch(context.Background(), sender, uid, job, source); err != nil {
		t.Fatal(err)
	}
	received, credential, err := ReceiveCredentialDispatch(context.Background(), receiver, uid)
	if err != nil || received != job || credential == nil {
		t.Fatal("credential dispatch lost", err)
	}
	data, err := ReadRepositoryPassword(context.Background(), credential)
	if err != nil || !bytes.Equal(data, secret) {
		t.Fatal("credential payload lost", err)
	}
	clear(data)
	credential.Close()
	data, err = ReadRepositoryPassword(context.Background(), source)
	if err != nil || !bytes.Equal(data, secret) {
		t.Fatal("receiver closed sender credential", err)
	}
	clear(data)
	sender2, receiver2 := dispatchPair(t)
	raw, err := EncodeDispatch(job)
	if err != nil {
		t.Fatal(err)
	}
	if err = disktransport.SendFile(context.Background(), sender2, raw, source); err != nil {
		t.Fatal(err)
	}
	if received, err = ReceiveDispatch(context.Background(), receiver2, uid); err == nil || received != (Dispatch{}) {
		t.Fatal("ordinary dispatch accepted credential")
	}
}

func TestCredentialDispatchReceiverRefusesForeignAndInvalidHandoff(t *testing.T) {
	uid := uint32(os.Geteuid())
	if uid == 0 {
		t.Skip("private peers must be unprivileged")
	}
	job := Dispatch{Version: 1, JobID: state.Random(), DeviceID: state.Random(), ManagementToken: state.Random(), RuntimeToken: state.Random(), Release: "0.1.0", CatalogVersion: 1}
	raw, err := EncodeDispatch(job)
	if err != nil {
		t.Fatal(err)
	}
	for _, scenario := range []string{"foreign-peer", "malformed-job", "disk-credential"} {
		t.Run(scenario, func(t *testing.T) {
			sender, receiver := dispatchPair(t)
			credential, err := CreateRepositoryPassword([]byte("fixture-only-credential"))
			if err != nil {
				t.Fatal(err)
			}
			defer credential.Close()
			expected := uid
			message := raw
			if scenario == "foreign-peer" {
				expected = uid + 1
			}
			if scenario == "malformed-job" {
				message = []byte(`{"version":1}`)
			}
			if scenario == "disk-credential" {
				path := t.TempDir() + "/credential"
				if err = os.WriteFile(path, []byte("fixture-only-credential"), 0600); err != nil {
					t.Fatal(err)
				}
				disk, err := os.Open(path)
				if err != nil {
					t.Fatal(err)
				}
				defer disk.Close()
				credential = disk
			}
			if err = disktransport.SendFile(context.Background(), sender, message, credential); err != nil {
				t.Fatal(err)
			}
			received, file, err := ReceiveCredentialDispatch(context.Background(), receiver, expected)
			if err == nil || file != nil || received != (Dispatch{}) {
				t.Fatal("unsafe credential handoff admitted")
			}
			if _, err = credential.Stat(); err != nil {
				t.Fatal("receiver invalidated sender handle", err)
			}
		})
	}
}

func TestActivatedCredentialSenderRefusesUnprivilegedCreator(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("fixture requires unprivileged listener creator")
	}
	sender, receiver := dispatchPair(t)
	job := Dispatch{Version: 1, JobID: state.Random(), DeviceID: state.Random(), ManagementToken: state.Random(), RuntimeToken: state.Random(), Release: "0.1.0", CatalogVersion: 1}
	credential, err := CreateRepositoryPassword([]byte("fixture-only-secret"))
	if err != nil {
		t.Fatal(err)
	}
	defer credential.Close()
	if err = SendActivatedCredentialDispatch(context.Background(), sender, job, credential); err == nil {
		t.Fatal("activated sender trusted unprivileged creator")
	}
	if err = receiver.SetReadDeadline(time.Now().Add(20 * time.Millisecond)); err != nil {
		t.Fatal(err)
	}
	data := make([]byte, MaxDispatchBytes)
	n, _, _, _, err := receiver.ReadMsgUnix(data, nil)
	timeout, ok := err.(net.Error)
	if n > 0 || !ok || !timeout.Timeout() {
		t.Fatal("refused activated sender exposed packet", n, err)
	}
}

func TestPreliminaryCredentialLaunchIsSeparateFromDispatch(t *testing.T) {
	uid := uint32(os.Geteuid())
	if uid == 0 {
		t.Skip("private peers must be unprivileged")
	}
	launch := Launch{Version: 2, JobID: state.Random(), DeviceID: state.Random(), ManagementToken: state.Random(), Release: "0.1.0", CatalogVersion: 1}
	raw, err := EncodeLaunch(launch)
	if err != nil {
		t.Fatal(err)
	}
	source, err := CreateRepositoryPassword([]byte("fixture-launch-secret"))
	if err != nil {
		t.Fatal(err)
	}
	defer source.Close()
	sender, receiver := dispatchPair(t)
	if err := sendCredentialPayload(context.Background(), sender, uid, raw, source); err != nil {
		t.Fatal(err)
	}
	got, credential, err := ReceiveCredentialLaunch(context.Background(), receiver, uid)
	if err != nil || got != launch || credential == nil {
		t.Fatal("launch handoff lost", got, err)
	}
	secret, err := ReadRepositoryPassword(context.Background(), credential)
	if err != nil || string(secret) != "fixture-launch-secret" {
		t.Fatal("launch credential lost", err)
	}
	clear(secret)
	credential.Close()
	sender2, receiver2 := dispatchPair(t)
	if err := sendCredentialPayload(context.Background(), sender2, uid, raw, source); err != nil {
		t.Fatal(err)
	}
	if dispatch, credential, err := ReceiveCredentialDispatch(context.Background(), receiver2, uid); err == nil || dispatch != (Dispatch{}) || credential != nil {
		t.Fatal("preliminary launch accepted as acquired dispatch")
	}
	if _, err := source.Stat(); err != nil {
		t.Fatal("receiver closed caller credential", err)
	}
}
