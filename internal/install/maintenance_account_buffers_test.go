package install

import (
	"bytes"
	"context"
	"testing"
)

type trackedAccountSnapshots struct {
	*fakeAccountProvisioner
	shadows [][]byte
}

func (b *trackedAccountSnapshots) Snapshot(ctx context.Context) (accountSnapshot, error) {
	snapshot, err := b.fakeAccountProvisioner.Snapshot(ctx)
	b.shadows = append(b.shadows, snapshot.shadow)
	return snapshot, err
}
func TestAccountPreparationClearsOwnedShadowBuffers(t *testing.T) {
	for _, collision := range []bool{false, true} {
		host, journal := roots(t)
		engine := openEngine(t, host, journal)
		source := newAccountFixture()
		if _, err := engine.provisionAccounts(context.Background(), source); err != nil {
			engine.Close()
			t.Fatal(err)
		}
		source.collide = collision
		tracked := &trackedAccountSnapshots{fakeAccountProvisioner: source}
		_, err := engine.prepareMaintenanceAccount(context.Background(), tracked)
		if (err != nil) != collision {
			engine.Close()
			t.Fatal("unexpected preparation result", err)
		}
		if len(tracked.shadows) == 0 {
			engine.Close()
			t.Fatal("snapshot not observed")
		}
		for _, buffer := range tracked.shadows {
			if !bytes.Equal(buffer, make([]byte, len(buffer))) {
				engine.Close()
				t.Fatal("owned shadow buffer retained")
			}
		}
		if !bytes.Contains(source.s.shadow, []byte("root:!:")) {
			engine.Close()
			t.Fatal("fixture source shadow modified")
		}
		engine.Close()
	}
}
