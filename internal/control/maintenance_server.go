package control

import (
	"context"
	"errors"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/pranavreddyg17/home-node/internal/supervisor"
)

// ServeMaintenance owns an already admitted private listener. Cancellation
// cancels request contexts, closes admission and drains active requests before
// returning. At most two app-maintenance operations can execute concurrently.
func (s *Server) ServeMaintenance(ctx context.Context, listener net.Listener, controllerUID, backupUID uint32) error {
	if listener == nil || controllerUID == 0 || backupUID == 0 || controllerUID == backupUID {
		return errors.New("invalid maintenance listener")
	}
	return serveMaintenance(ctx, listener, s.MaintenanceHandler(controllerUID, backupUID))
}
func serveMaintenance(ctx context.Context, listener net.Listener, handler http.Handler) error {
	defer listener.Close()
	serverContext, cancel := context.WithCancel(ctx)
	defer cancel()
	var admission sync.Mutex
	var handlers sync.WaitGroup
	slots := make(chan struct{}, 2)
	server := &http.Server{BaseContext: func(net.Listener) context.Context { return serverContext }, ConnContext: supervisor.PeerContext, ReadHeaderTimeout: 3 * time.Second, ReadTimeout: 10 * time.Second, WriteTimeout: 185 * time.Second, IdleTimeout: 15 * time.Second, MaxHeaderBytes: 1024, Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		admission.Lock()
		if serverContext.Err() != nil {
			admission.Unlock()
			http.Error(w, "maintenance stopping", 503)
			return
		}
		select {
		case slots <- struct{}{}:
			handlers.Add(1)
			admission.Unlock()
			defer handlers.Done()
			defer func() { <-slots }()
			handler.ServeHTTP(w, r)
		default:
			admission.Unlock()
			http.Error(w, "maintenance busy", 503)
		}
	})}
	shutdownDone := make(chan error, 1)
	go func() {
		<-serverContext.Done()
		deadline, stop := context.WithTimeout(context.Background(), 10*time.Second)
		defer stop()
		err := server.Shutdown(deadline)
		if err != nil {
			err = errors.Join(err, server.Close())
		}
		shutdownDone <- err
	}()
	err := server.Serve(listener)
	cancel()
	if errors.Is(err, http.ErrServerClosed) {
		err = nil
	}
	shutdownErr := <-shutdownDone
	// Synchronize enrollment before waiting: cancellation forbids new handlers.
	admission.Lock()
	admission.Unlock()
	handlers.Wait()
	return errors.Join(err, shutdownErr)
}
