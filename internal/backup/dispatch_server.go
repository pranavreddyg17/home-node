package backup

import (
	"context"
	"errors"
	"net"
	"sync"
	"time"
)

// ServeDispatch owns a private unixpacket listener and admits at most one
// received/running job. Overflow connections are closed without reading tokens.
// Cancellation or listener failure cancels and joins active work before return.
// A worker failure stops the service; the coordinator must reconcile its owned
// job rather than treating socket delivery or process restart as backup success.
// Work must persist repository outcomes through the authenticated controller.
func ServeDispatch(ctx context.Context, listener net.Listener, controllerUID uint32, work func(context.Context, Dispatch) error) (resultErr error) {
	if listener == nil {
		return ErrManifest
	}
	if controllerUID == 0 || work == nil || listener.Addr().Network() != "unixpacket" {
		_ = listener.Close()
		return ErrManifest
	}
	serving, cancel := context.WithCancel(ctx)
	closed := make(chan struct{})
	stop := context.AfterFunc(serving, func() { _ = listener.Close(); close(closed) })
	var workers sync.WaitGroup
	slot := make(chan struct{}, 1)
	failure := make(chan error, 1)
	defer func() {
		cancel()
		_ = listener.Close()
		if !stop() {
			<-closed
		}
		workers.Wait()
		select {
		case err := <-failure:
			resultErr = errors.Join(resultErr, err)
		default:
		}
	}()
	for {
		connection, err := listener.Accept()
		if err != nil {
			if serving.Err() != nil {
				return nil
			}
			return err
		}
		if serving.Err() != nil {
			_ = connection.Close()
			return nil
		}
		packet, ok := connection.(*net.UnixConn)
		if !ok {
			_ = connection.Close()
			continue
		}
		select {
		case slot <- struct{}{}:
		default:
			_ = packet.Close()
			continue
		}
		workers.Add(1)
		go func() {
			defer workers.Done()
			defer func() { <-slot }()
			defer packet.Close()
			dispatch, err := ReceiveDispatch(serving, packet, controllerUID)
			if err != nil {
				return
			}
			operation, end := context.WithTimeout(serving, 2*time.Hour)
			defer end()
			err = work(operation, dispatch)
			if err != nil && serving.Err() == nil {
				failure <- err
				cancel()
			}
		}()
	}
}
