package workload

import (
	"context"
	"errors"
	"testing"

	"github.com/pranavreddyg17/home-node/internal/state"
)

func TestAppActionRejectsRevisionOverflowWithoutLosingReplay(t *testing.T) {
	for _, revision := range []int64{-1, 1 << 53, 1<<63 - 1} {
		s, _, device := service(t)
		ctx := context.Background()
		key := state.Random()
		op, err := s.AppAction(ctx, device, key, "files", "start")
		if err != nil {
			t.Fatal(err)
		}
		if _, err = s.Store.DB.Exec("UPDATE apps SET revision=?,state='failed' WHERE workload='files'", revision); err != nil {
			t.Fatal(err)
		}
		for _, action := range []string{"start", "stop"} {
			if _, err = s.AppAction(ctx, device, state.Random(), "files", action); !errors.Is(err, ErrConflict) {
				t.Fatal("invalid revision admitted", revision, action, err)
			}
		}
		replay, err := s.AppAction(ctx, device, key, "files", "start")
		if err != nil || replay.ID != op.ID {
			t.Fatal("invalid metadata hid existing operation", replay, err)
		}
		var count int
		if err = s.Store.DB.QueryRow("SELECT count(*) FROM operations").Scan(&count); err != nil || count != 1 {
			t.Fatal("rejected command retained intent", count, err)
		}
	}
}
