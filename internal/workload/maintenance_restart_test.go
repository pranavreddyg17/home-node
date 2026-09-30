package workload

import (
	"context"
	"errors"
	"testing"

	"github.com/pranavreddyg17/home-node/internal/state"
)

func TestMaintenanceRestartRestoresOnlyItsStoppedAppAndRetainsAdmission(t *testing.T) {
	s, b, device := service(t)
	ctx := context.Background()
	startFiles(t, s, device)
	token, err := s.Store.BeginMaintenance(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.MaintenanceRestart(ctx, token, device, "files"); err == nil {
		t.Fatal("restart without owned stop accepted")
	}
	stop, err := s.MaintenanceStop(ctx, token, device, "files")
	if err != nil {
		t.Fatal(err)
	}
	if err = s.processApp(ctx); err != nil {
		t.Fatal(err)
	}
	restart, err := s.MaintenanceRestart(ctx, token, device, "files")
	if err != nil {
		t.Fatal(err)
	}
	if restart.Kind != "app.start" || restart.ID == stop.ID {
		t.Fatal(restart)
	}
	replay, err := s.MaintenanceRestart(ctx, token, device, "files")
	if err != nil || replay.ID != restart.ID {
		t.Fatal(replay, err)
	}
	if err = s.processApp(ctx); err != nil {
		t.Fatal(err)
	}
	result, err := s.Operation(ctx, device, restart.ID)
	if err != nil || result.State != "succeeded" || b.starts != 2 {
		t.Fatal(result, b.starts, err)
	}
	if _, err = s.CreateConversation(ctx, "still blocked"); !errors.Is(err, state.ErrMaintenance) {
		t.Fatal("restart reopened admission", err)
	}
	if err = s.Store.EndMaintenance(ctx, token); err != nil {
		t.Fatal(err)
	}
}

func TestMaintenanceRestartRefusesChangedOwnershipOrAuthority(t *testing.T) {
	for _, scenario := range []string{"revoked", "superseded", "stop-failed", "foreign-token", "activity"} {
		t.Run(scenario, func(t *testing.T) {
			s, b, device := service(t)
			ctx := context.Background()
			startFiles(t, s, device)
			token, err := s.Store.BeginMaintenance(ctx)
			if err != nil {
				t.Fatal(err)
			}
			stop, err := s.MaintenanceStop(ctx, token, device, "files")
			if err != nil {
				t.Fatal(err)
			}
			if err = s.processApp(ctx); err != nil {
				t.Fatal(err)
			}
			switch scenario {
			case "revoked":
				_, err = s.Store.DB.Exec("UPDATE devices SET revoked_at=1 WHERE id=?", device)
			case "superseded":
				_, err = s.Store.DB.Exec("UPDATE apps SET operation_id=?,revision=revision+1 WHERE workload='files'", state.Random())
			case "stop-failed":
				_, err = s.Store.DB.Exec("UPDATE operations SET state='failed' WHERE id=?", stop.ID)
			case "foreign-token":
				token = state.Random()
			case "activity":
				_, err = s.Store.DB.Exec("INSERT INTO settings VALUES('host.activity.fixture','trash-expiry')")
			}
			if err != nil {
				t.Fatal(err)
			}
			var before, after int
			if err = s.Store.DB.QueryRow("SELECT count(*) FROM operations").Scan(&before); err != nil {
				t.Fatal(err)
			}
			if _, err = s.MaintenanceRestart(ctx, token, device, "files"); err == nil {
				t.Fatal("invalid restart admitted")
			}
			if err = s.Store.DB.QueryRow("SELECT count(*) FROM operations").Scan(&after); err != nil || before != after || b.starts != 1 {
				t.Fatal("rejected restart retained effects", before, after, b.starts, err)
			}
		})
	}
}
