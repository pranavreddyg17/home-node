package backup

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestLaunchReplyDomainsSeparateStoppedRefusalFromSuccessfulPublication(t *testing.T) {
	launch := Launch{Version: 2, JobID: strings.Repeat("a", 24), DeviceID: strings.Repeat("b", 24), ManagementToken: strings.Repeat("c", 24), Release: "0.1.0", CatalogVersion: 1}
	if err := parseLaunchCompletionPacket(launchCompletionPacket(launch), launch); err != nil {
		t.Fatal(err)
	}
	err := parseLaunchCompletionPacket(launchRepositoryRefusalPacket(launch), launch)
	if !errors.Is(err, ErrLaunchRepositoryRefusedStopped) || errors.Is(err, ErrLaunchRepositoryAdmission) {
		t.Fatal("refusal confused with local error", err)
	}
	foreign := launch
	foreign.JobID = strings.Repeat("d", 24)
	for _, raw := range [][]byte{launchRepositoryRefusalPacket(foreign), []byte("homenode-backup-complete-v1 " + launch.JobID), []byte("homenode-backup-cleanup-complete-v3 " + launch.JobID), append(launchRepositoryRefusalPacket(launch), '\n'), append(launchRepositoryRefusalPacket(launch), []byte(" secret")...), nil} {
		if err := parseLaunchCompletionPacket(raw, launch); !errors.Is(err, ErrManifest) {
			t.Fatal("wrong reply domain accepted", err)
		}
	}
	invalid := launch
	invalid.Version = 1
	if err := parseLaunchCompletionPacket(launchRepositoryRefusalPacket(invalid), invalid); !errors.Is(err, ErrManifest) {
		t.Fatal("invalid launch admitted", err)
	}
	// Refuse before touching a connection; ordinary failure is not stop evidence.
	for _, cause := range []error{nil, context.Canceled, ErrRepository, ErrManifest} {
		if err := sendLaunchRepositoryRefusal(context.Background(), nil, launch, cause); !errors.Is(err, ErrManifest) {
			t.Fatal("unqualified failure upgraded", err)
		}
	}
}
