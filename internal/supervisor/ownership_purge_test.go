package supervisor

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/pranavreddyg17/home-node/internal/state"
)

func TestLegacyPurgeRefusesReservedOwnershipEvidence(t *testing.T) {
	for _, evidence := range []string{"lease", "group", "intent"} {
		t.Run(evidence, func(t *testing.T) {
			m, backend := newManager(t)
			id := state.Random()
			if _, err := m.Store.DB.Exec(`INSERT INTO runtime_instances(id,workload,state,desired,image_sha256,memory_mib,vcpus,data_bytes,created_at,revision) VALUES(?,'video','stopped','stopped',?,256,1,?,0,1)`, id, m.Manifest.Images[0].SHA256, 16<<20); err != nil {
				t.Fatal(err)
			}
			switch evidence {
			case "lease":
				_, err := m.Store.DB.Exec(`INSERT INTO runtime_uid_leases VALUES(?,200000)`, id)
				if err != nil {
					t.Fatal(err)
				}
			case "group":
				_, err := m.Store.DB.Exec(`INSERT INTO runtime_guest_groups VALUES(?,64055)`, id)
				if err != nil {
					t.Fatal(err)
				}
			case "intent":
				_, err := m.Store.DB.Exec(`INSERT INTO runtime_volume_ownership VALUES(?,?,200000,64055,10,100,?)`, id, m.Manifest.Images[0].SHA256, 16<<20)
				if err != nil {
					t.Fatal(err)
				}
			}
			path := filepath.Join(m.Volumes, id+".raw")
			if err := os.WriteFile(path, []byte("retained-volume"), 0600); err != nil {
				t.Fatal(err)
			}
			backend.cleanupErr = errors.New("cleanup must not run")
			_, err := m.Apply(context.Background(), Request{Version: 1, OperationID: state.Random(), InstanceID: id, Action: "purge", Revision: 2, PolicyGeneration: m.Policy.Generation})
			if !errors.Is(err, ErrPolicy) {
				t.Fatal("legacy purge admitted reserved evidence", err)
			}
			data, err := os.ReadFile(path)
			if err != nil || string(data) != "retained-volume" {
				t.Fatal("purge changed volume", string(data), err)
			}
			var phase string
			if err := m.Store.DB.QueryRow(`SELECT state FROM runtime_instances WHERE id=?`, id).Scan(&phase); err != nil || phase != "stopped" {
				t.Fatal("purge changed state", phase, err)
			}
			var operations int
			if err := m.Store.DB.QueryRow(`SELECT count(*) FROM runtime_operations`).Scan(&operations); err != nil || operations != 0 {
				t.Fatal("purge admitted operation", operations, err)
			}
		})
	}
}
