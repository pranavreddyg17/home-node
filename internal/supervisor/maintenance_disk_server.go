package supervisor

import (
	"context"
	"net"
	"sync"
)

// ServeMaintenanceDisks owns the listener and admitted connections. At most two
// connections can occupy admission/copy work; excess connections are closed.
// Cancellation closes admission and waits for bounded active handlers to exit.
func (m *Manager) ServeMaintenanceDisks(ctx context.Context, listener *net.UnixListener, backupUID uint32) error {
	return m.serveMaintenanceDisks(ctx, listener, backupUID, m.ServeMaintenanceDisk)
}
func (m *Manager) serveMaintenanceDisks(ctx context.Context, listener *net.UnixListener, backupUID uint32, handle func(context.Context, *net.UnixConn, uint32) error) error {
	if listener == nil || handle == nil {
		return ErrPolicy
	}
	defer listener.Close()
	if backupUID == 0 || backupUID == m.Policy.ControllerUID || backupUID == m.Policy.TransferUID {
		return ErrPolicy
	}
	workerContext, cancelWorkers := context.WithCancel(ctx)
	stopped := make(chan struct{})
	stop := context.AfterFunc(workerContext, func() { _ = listener.Close(); close(stopped) })
	defer func() {
		if !stop() {
			<-stopped
		}
	}()
	handlers := sync.WaitGroup{}
	defer func() { cancelWorkers(); handlers.Wait() }()
	slots := make(chan struct{}, 2)
	for {
		connection, err := listener.AcceptUnix()
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			return err
		}
		select {
		case slots <- struct{}{}:
			handlers.Add(1)
			go func() {
				defer handlers.Done()
				defer connection.Close()
				defer func() { <-slots }()
				_ = handle(workerContext, connection, backupUID)
			}()
		default:
			_ = connection.Close()
		}
	}
}
