package control

import (
	"context"
	"github.com/pranavreddyg17/home-node/internal/backup"
	"github.com/pranavreddyg17/home-node/internal/identity"
	"github.com/pranavreddyg17/home-node/internal/state"
	"time"
)

func (s *Server) beginSnapshotPreviewRequest(ctx context.Context, actor identity.Session, snapshot string) (backup.SnapshotPreviewRequest, func(), error) {
	if err := s.Identity.VerifyBackupSnapshotSession(ctx, actor); err != nil {
		return backup.SnapshotPreviewRequest{}, nil, err
	}
	request := backup.SnapshotPreviewRequest{Version: 5, Kind: "snapshot-preview", RequestID: state.Random(), DeviceID: actor.Device.ID, SnapshotID: snapshot}
	if _, err := backup.EncodeSnapshotPreviewRequest(request); err != nil {
		return backup.SnapshotPreviewRequest{}, nil, err
	}
	operation, cancel := context.WithTimeout(ctx, 2*time.Minute)
	actor.Device.Capabilities = append([]string(nil), actor.Device.Capabilities...)
	entry := &pendingSnapshotRequest{preview: request, actor: actor, ctx: operation, cancel: cancel}
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
		return backup.SnapshotPreviewRequest{}, nil, identity.ErrDenied
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

func (s *Server) snapshotPreviewOperationContext(request backup.SnapshotPreviewRequest) (context.Context, error) {
	r := &s.snapshotRequests
	r.mu.Lock()
	defer r.mu.Unlock()
	entry := r.pending[request.RequestID]
	if r.stopping || entry == nil || entry.preview != request || entry.ctx.Err() != nil {
		return nil, identity.ErrDenied
	}
	return entry.ctx, nil
}

func (s *Server) verifySnapshotPreviewRequest(ctx context.Context, request backup.SnapshotPreviewRequest) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	r := &s.snapshotRequests
	r.mu.Lock()
	entry := r.pending[request.RequestID]
	valid := entry != nil && entry.preview == request && entry.ctx.Err() == nil
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
