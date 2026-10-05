package control

import (
	"context"
	"sync"
	"time"

	"github.com/pranavreddyg17/home-node/internal/backup"
	"github.com/pranavreddyg17/home-node/internal/identity"
	"github.com/pranavreddyg17/home-node/internal/state"
)

type pendingSnapshotRequest struct {
	preview backup.SnapshotPreviewRequest
	request backup.SnapshotPageRequest
	actor   identity.Session
	ctx     context.Context
	cancel  context.CancelFunc
}

// Pending metadata requests are process-local: controller restart withdraws
// them. No session hash or credential crosses into the worker request.
type snapshotRequests struct {
	mu       sync.Mutex
	pending  map[string]*pendingSnapshotRequest
	stopping bool
}

func (s *Server) beginSnapshotRequest(ctx context.Context, actor identity.Session, cursor string) (backup.SnapshotPageRequest, func(), error) {
	if err := s.Identity.VerifyBackupSnapshotSession(ctx, actor); err != nil {
		return backup.SnapshotPageRequest{}, nil, err
	}
	request := backup.SnapshotPageRequest{Version: 4, Kind: "snapshot-page", RequestID: state.Random(), DeviceID: actor.Device.ID, Cursor: cursor}
	if _, err := backup.EncodeSnapshotPageRequest(request); err != nil {
		return backup.SnapshotPageRequest{}, nil, err
	}
	operation, cancel := context.WithTimeout(ctx, 2*time.Minute)
	actor.Device.Capabilities = append([]string(nil), actor.Device.Capabilities...)
	entry := &pendingSnapshotRequest{request: request, actor: actor, ctx: operation, cancel: cancel}
	r := &s.snapshotRequests
	r.mu.Lock()
	if r.pending == nil {
		r.pending = make(map[string]*pendingSnapshotRequest)
	}
	for id, pending := range r.pending {
		if pending.ctx.Err() != nil {
			delete(r.pending, id)
		}
	}
	if r.stopping || len(r.pending) >= 4 || operation.Err() != nil {
		r.mu.Unlock()
		cancel()
		return backup.SnapshotPageRequest{}, nil, identity.ErrDenied
	}
	r.pending[request.RequestID] = entry
	r.mu.Unlock()
	release := func() {
		cancel()
		r.mu.Lock()
		if r.pending[request.RequestID] == entry {
			delete(r.pending, request.RequestID)
		}
		r.mu.Unlock()
	}
	return request, release, nil
}

func (s *Server) snapshotOperationContext(request backup.SnapshotPageRequest) (context.Context, error) {
	r := &s.snapshotRequests
	r.mu.Lock()
	defer r.mu.Unlock()
	entry := r.pending[request.RequestID]
	if r.stopping || entry == nil || entry.request != request || entry.ctx.Err() != nil {
		return nil, identity.ErrDenied
	}
	return entry.ctx, nil
}

func (s *Server) closeSnapshotRequests() {
	r := &s.snapshotRequests
	r.mu.Lock()
	defer r.mu.Unlock()
	r.stopping = true
	for id, entry := range r.pending {
		entry.cancel()
		delete(r.pending, id)
	}
}

func (s *Server) verifySnapshotRequest(ctx context.Context, request backup.SnapshotPageRequest) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	r := &s.snapshotRequests
	r.mu.Lock()
	entry := r.pending[request.RequestID]
	valid := entry != nil && entry.request == request && entry.ctx.Err() == nil
	r.mu.Unlock()
	if !valid {
		return identity.ErrDenied
	}
	if err := s.Identity.VerifyBackupSnapshotSession(ctx, entry.actor); err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.pending[request.RequestID] != entry || entry.ctx.Err() != nil {
		return identity.ErrDenied
	}
	return ctx.Err()
}
