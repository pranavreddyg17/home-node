package supervisor

import (
	"context"
	"time"
)

// lockStart bounds waiting for serialized preparation. A cancelled waiter must
// release its outer runtime admission instead of delaying maintenance forever.
func (m *Manager) lockStart(ctx context.Context) (func(), error) {
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if m.startMu.TryLock() {
			if err := ctx.Err(); err != nil {
				m.startMu.Unlock()
				return nil, err
			}
			return m.startMu.Unlock, nil
		}
		timer := time.NewTimer(10 * time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil, ctx.Err()
		case <-timer.C:
		}
	}
}
