package workload

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/pranavreddyg17/home-node/internal/state"
)

func TestMaintenanceRestorationCanResumeAfterCancelledWait(t *testing.T) {
	s, b, device := service(t)
	ctx := context.Background()
	startFiles(t, s, device)
	token, err := s.Store.BeginMaintenance(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.MaintenanceStop(ctx, token, device, "files"); err != nil {
		t.Fatal(err)
	}
	if err = s.processApp(ctx); err != nil {
		t.Fatal(err)
	}
	wait, cancel := context.WithTimeout(ctx, 80*time.Millisecond)
	defer cancel()
	if err = s.RestoreMaintenanceApps(wait, token, device); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal(err)
	}
	if _, err = s.Store.InspectMaintenance(ctx, token); err != nil {
		t.Fatal("cancel released barrier", err)
	}
	if b.starts != 1 {
		t.Fatal("restore bypassed worker", b.starts)
	}
	if err = s.processAppKind(ctx, "app.start"); err != nil {
		t.Fatal(err)
	}
	if err = s.RestoreMaintenanceApps(ctx, token, device); err != nil {
		t.Fatal(err)
	}
	if b.starts != 2 {
		t.Fatal("restore duplicated runtime effect", b.starts)
	}
	if _, err = s.CreateConversation(ctx, "still blocked"); !errors.Is(err, state.ErrMaintenance) {
		t.Fatal("restore reopened admission", err)
	}
}

func TestMaintenanceRestorationRejectsFailedOrSupersededRestart(t *testing.T) {
	for _, scenario := range []string{"failed", "superseded", "foreign-device", "revoked", "superseded-pending"} {
		t.Run(scenario, func(t *testing.T) {
			s, _, device := service(t)
			ctx := context.Background()
			startFiles(t, s, device)
			token, err := s.Store.BeginMaintenance(ctx)
			if err != nil {
				t.Fatal(err)
			}
			if _, err = s.MaintenanceStop(ctx, token, device, "files"); err != nil {
				t.Fatal(err)
			}
			if err = s.processApp(ctx); err != nil {
				t.Fatal(err)
			}
			op, err := s.MaintenanceRestart(ctx, token, device, "files")
			if err != nil {
				t.Fatal(err)
			}
			if scenario == "foreign-device" {
				other := state.Random()
				if _, err = s.Store.DB.Exec("INSERT INTO devices(id,name,capabilities,created_at) VALUES(?,'other','[\"admin\"]',1)", other); err != nil {
					t.Fatal(err)
				}
				device = other
			} else if scenario == "revoked" {
				_, err = s.Store.DB.Exec("UPDATE devices SET revoked_at=1 WHERE id=?", device)
			} else if scenario == "superseded-pending" {
				_, err = s.Store.DB.Exec("UPDATE apps SET operation_id=?,revision=revision+1 WHERE workload='files'", state.Random())
			} else if scenario == "failed" {
				_, err = s.Store.DB.Exec("UPDATE operations SET state='failed' WHERE id=?", op.ID)
			} else {
				if err = s.processApp(ctx); err != nil {
					t.Fatal(err)
				}
				_, err = s.Store.DB.Exec("UPDATE apps SET operation_id=?,revision=revision+1 WHERE workload='files'", state.Random())
			}
			if err != nil {
				t.Fatal(err)
			}
			if err = s.RestoreMaintenanceApps(ctx, token, device); !errors.Is(err, ErrConflict) {
				t.Fatal("restoration falsely confirmed", err)
			}
			if _, err = s.Store.InspectMaintenance(ctx, token); err != nil {
				t.Fatal("failure released barrier", err)
			}
		})
	}
}
