//go:build linux

package backup

import (
	"context"
	"errors"
	"testing"

	"github.com/pranavreddyg17/home-node/internal/state"
)

func TestLaunchRefusalReplyRequiresOwnJobAndUncancelledReceipt(t *testing.T) {
	launch := Launch{Version: 2, JobID: state.Random(), DeviceID: state.Random(), ManagementToken: state.Random(), Release: "0.1.0", CatalogVersion: 1}
	for _, scenario := range []string{"refused", "foreign-job", "cancelled"} {
		t.Run(scenario, func(t *testing.T) {
			sender, receiver := dispatchPair(t)
			replied := launch
			if scenario == "foreign-job" {
				replied.JobID = state.Random()
			}
			cause := errors.Join(ErrLaunchRepositoryAdmission, errors.New("private repository detail"))
			if err := sendLaunchRepositoryRefusal(context.Background(), receiver, replied, cause); err != nil {
				t.Fatal(err)
			}
			ctx := context.Background()
			if scenario == "cancelled" {
				var cancel context.CancelFunc
				ctx, cancel = context.WithCancel(ctx)
				cancel()
			}
			err := waitLaunchCompletion(ctx, sender, launch)
			switch scenario {
			case "refused":
				if !errors.Is(err, ErrLaunchRepositoryRefusedStopped) || errors.Is(err, ErrLaunchRepositoryAdmission) {
					t.Fatal("qualified refusal lost", err)
				}
			case "foreign-job":
				if !errors.Is(err, ErrManifest) {
					t.Fatal("foreign refusal admitted", err)
				}
			case "cancelled":
				if !errors.Is(err, context.Canceled) || errors.Is(err, ErrLaunchRepositoryRefusedStopped) {
					t.Fatal("cancelled receipt upgraded to stop evidence", err)
				}
			}
		})
	}
}
