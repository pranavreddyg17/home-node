package backup

import (
	"context"
	"errors"
	"net"
	"os"
	"sync"
	"time"
)

// ServeDispatch owns a private unixpacket listener and admits at most one
// received/running job. Overflow connections are closed without reading tokens.
// Cancellation or listener failure cancels and joins active work before return.
// A worker failure stops the service; the coordinator must reconcile its owned
// job rather than treating socket delivery or process restart as backup success.
// Work must persist repository outcomes through the authenticated controller.
func ServeDispatch(ctx context.Context, listener net.Listener, controllerUID uint32, work func(context.Context, Dispatch) error) error {
	var operation func(context.Context, Dispatch, *os.File) error
	if work != nil {
		operation = func(ctx context.Context, dispatch Dispatch, _ *os.File) error { return work(ctx, dispatch) }
	}
	return serveDispatch(ctx, listener, controllerUID, func(ctx context.Context, connection *net.UnixConn, uid uint32) (Dispatch, *os.File, error) {
		dispatch, err := ReceiveDispatch(ctx, connection, uid)
		return dispatch, nil, err
	}, operation, nil)
}

// ServeCredentialDispatch belongs on the separate private credential channel.
// It uses the same single-job admission and joined shutdown lifecycle. Received
// credentials are closed on every exit, including cancellation before work.
// A callback may consume/close its credential earlier; no descriptor is retained
// after its return. Delivery and callback completion are not publication proof.
func ServeCredentialDispatch(ctx context.Context, listener net.Listener, controllerUID uint32, work func(context.Context, Dispatch, *os.File) error) error {
	return serveDispatch(ctx, listener, controllerUID, ReceiveCredentialDispatch, work, nil)
}

// ServeAcknowledgedCredentialDispatch acknowledges only successful callback
// return after received credential closure. Publication still requires durable
// outcome inspection; lost acknowledgements must never trigger job replay.
func ServeAcknowledgedCredentialDispatch(ctx context.Context, listener net.Listener, controllerUID uint32, work func(context.Context, Dispatch, *os.File) error) error {
	return serveDispatch(ctx, listener, controllerUID, ReceiveCredentialDispatch, work, sendWorkerCompletion)
}

func serveDispatch[T any](ctx context.Context, listener net.Listener, controllerUID uint32, receive func(context.Context, *net.UnixConn, uint32) (T, *os.File, error), work func(context.Context, T, *os.File) error, complete func(context.Context, *net.UnixConn, T) error) (resultErr error) {
	if listener == nil {
		return ErrManifest
	}
	if controllerUID == 0 || work == nil || receive == nil || listener.Addr().Network() != "unixpacket" {
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
			dispatch, credential, err := receive(serving, packet, controllerUID)
			if credential != nil {
				defer credential.Close()
			}
			if err != nil {
				return
			}
			operation, end := context.WithTimeout(serving, 2*time.Hour)
			defer end()
			if operation.Err() != nil {
				return
			}
			err = work(operation, dispatch, credential)
			if complete != nil && err == nil {
				if credential != nil {
					closeErr := credential.Close()
					if closeErr != nil && !errors.Is(closeErr, os.ErrClosed) {
						err = closeErr
					}
				}
				if err == nil {
					err = complete(operation, packet, dispatch)
				}
			}
			if err != nil && serving.Err() == nil {
				failure <- err
				cancel()
			}
		}()
	}
}

// ServeAcknowledgedCredentialLaunch admits only preliminary launch messages.
// It shares joined single-job lifecycle and closes credentials before completion.
func ServeAcknowledgedCredentialLaunch(ctx context.Context, listener net.Listener, controllerUID uint32, work func(context.Context, Launch, *os.File) error) error {
	return serveDispatch(ctx, listener, controllerUID, ReceiveCredentialLaunch, work, sendLaunchCompletion)
}

// ServeAcknowledgedCredentialCleanup accepts only explicit stopped-job cleanup.
// The callback must independently qualify stopped/restoring ownership before
// releasing root authority; credential possession alone is never sufficient.
func ServeAcknowledgedCredentialCleanup(ctx context.Context, listener net.Listener, controllerUID uint32, work func(context.Context, Cleanup, *os.File) error) error {
	return serveDispatch(ctx, listener, controllerUID, ReceiveCredentialCleanup, work, sendCleanupCompletion)
}
