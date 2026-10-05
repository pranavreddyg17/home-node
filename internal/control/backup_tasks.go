package control

import (
	"context"
	"errors"
	"log/slog"
	"sync"
)

var errBackupTaskUnavailable = errors.New("backup task admission unavailable")

// backupTasks owns one job independently of its initiating HTTP connection.
// Admission and shutdown serialize; shutdown never races WaitGroup enrollment.
type backupTasks struct {
	mu               sync.Mutex
	context          context.Context
	cancel           context.CancelFunc
	active, stopping bool
	workers          sync.WaitGroup
}

func newBackupTasks() *backupTasks {
	ctx, cancel := context.WithCancel(context.Background())
	return &backupTasks{context: ctx, cancel: cancel}
}

// start invokes the durable admission callback while holding task admission.
// On success run owns all resources captured by its closure, including credential
// cleanup. The callback must never return nil work after committing admission.
func (tasks *backupTasks) start(admit func(context.Context) (func(context.Context) error, error)) error {
	tasks.mu.Lock()
	defer tasks.mu.Unlock()
	if tasks.stopping || tasks.active || admit == nil {
		return errBackupTaskUnavailable
	}
	run, err := admit(tasks.context)
	if err != nil {
		return err
	}
	if run == nil {
		return errBackupTaskUnavailable
	}
	tasks.active = true
	tasks.workers.Add(1)
	go func() {
		defer tasks.workers.Done()
		defer func() { tasks.mu.Lock(); tasks.active = false; tasks.mu.Unlock() }()
		if err := run(tasks.context); err != nil {
			slog.Error("Backup job ended without complete acknowledgement; inspect durable backup status")
		}
	}()
	return nil
}
func (tasks *backupTasks) close(ctx context.Context) error {
	tasks.mu.Lock()
	tasks.stopping = true
	tasks.cancel()
	tasks.mu.Unlock()
	done := make(chan struct{})
	go func() { tasks.workers.Wait(); close(done) }()
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// CloseBackupWork cancels server-owned work and waits for coordinator cleanup.
// A timeout does not imply worker completion or permit closing active resources.
func (s *Server) CloseBackupWork(ctx context.Context) error {
	s.closeSnapshotRequests()
	return s.backupTasks.close(ctx)
}
