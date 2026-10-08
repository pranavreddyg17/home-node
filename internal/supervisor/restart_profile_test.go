package supervisor

import (
	"context"
	"errors"
	"testing"

	"github.com/pranavreddyg17/home-node/internal/state"
)

func TestRestartRefusesRecordedResourceProfileDrift(t *testing.T) {
	for _, field := range []string{"memory", "vcpus", "size"} {
		t.Run(field, func(t *testing.T) {
			m, backend := newManager(t)
			ctx := context.Background()
			start := startRequest()
			if _, err := m.Apply(ctx, start); err != nil {
				t.Fatal(err)
			}
			if _, err := m.Apply(ctx, Request{Version: 1, OperationID: state.Random(), InstanceID: start.InstanceID, Action: "stop", Revision: 2, PolicyGeneration: m.Policy.Generation}); err != nil {
				t.Fatal(err)
			}
			var query string
			switch field {
			case "memory":
				query = `UPDATE runtime_instances SET memory_mib=memory_mib+1 WHERE id=?`
			case "vcpus":
				query = `UPDATE runtime_instances SET vcpus=vcpus+1 WHERE id=?`
			case "size":
				query = `UPDATE runtime_instances SET data_bytes=data_bytes+1 WHERE id=?`
			}
			if _, err := m.Store.DB.Exec(query, start.InstanceID); err != nil {
				t.Fatal(err)
			}
			retry := start
			retry.OperationID = state.Random()
			retry.Revision = 3
			if _, err := m.Apply(ctx, retry); !errors.Is(err, ErrPolicy) {
				t.Fatal("restart accepted profile drift", err)
			}
			if backend.starts != 1 || backend.running {
				t.Fatal("restart reached backend", backend.starts, backend.running)
			}
			var phase, desired string
			if err := m.Store.DB.QueryRow(`SELECT state,desired FROM runtime_instances WHERE id=?`, start.InstanceID).Scan(&phase, &desired); err != nil || phase != "stopped" || desired != "stopped" {
				t.Fatal("restart changed stopped inventory", phase, desired, err)
			}
			var pending int
			if err := m.Store.DB.QueryRow(`SELECT count(*) FROM runtime_operations WHERE id=?`, retry.OperationID).Scan(&pending); err != nil || pending != 0 {
				t.Fatal("refused restart recorded operation", pending, err)
			}
		})
	}
}
