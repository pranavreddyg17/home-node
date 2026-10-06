// Package transfer mediates bounded guest bytes without runtime authority or
// access to management state. The API UID cannot open guest channels directly.
package transfer

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"hash/fnv"
	"io"
	"net"
	"net/http"
	"path/filepath"
	"time"

	"github.com/pranavreddyg17/home-node/internal/guestproto"
	"github.com/pranavreddyg17/home-node/internal/runtimeclient"
	"github.com/pranavreddyg17/home-node/internal/state"
	"github.com/pranavreddyg17/home-node/internal/supervisor"
)

type Inspector interface {
	Apply(context.Context, supervisor.Request) (supervisor.Instance, error)
}
type Service struct {
	SharedGuestUID   uint32
	Runtime          Inspector
	Channels         string
	ControllerUID    uint32
	PolicyGeneration int64
	locks            [16]chan struct{}
	connections      [16]net.Conn
	instanceIDs      [16]string
}

func New(runtime Inspector, channels string, uid uint32, generation int64) *Service {
	s := &Service{Runtime: runtime, Channels: channels, ControllerUID: uid, PolicyGeneration: generation}
	for i := range s.locks {
		s.locks[i] = make(chan struct{}, 1)
	}
	return s
}
func (s *Service) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	uid, ok := supervisor.RequestPeerUID(r)
	if !ok || uid != s.ControllerUID {
		http.Error(w, "peer denied", 403)
		return
	}
	if r.Method != "POST" || r.URL.Path != "/v1/guest" {
		http.NotFound(w, r)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, guestproto.MaxFrame)
	var request runtimeclient.GuestRequest
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if decoder.Decode(&request) != nil || decoder.Decode(new(any)) != io.EOF || !guestproto.ValidID(request.InstanceID) || guestproto.Validate(request.Request) != nil {
		http.Error(w, "invalid request", 400)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancel()
	hash := fnv.New32a()
	_, _ = hash.Write([]byte(request.InstanceID))
	index := hash.Sum32() % 16
	lock := s.locks[index]
	select {
	case lock <- struct{}{}:
		defer func() { <-lock }()
	case <-ctx.Done():
		http.Error(w, "busy", 503)
		return
	}
	instance, err := s.Runtime.Apply(ctx, supervisor.Request{Version: 1, OperationID: state.Random(), Action: "inspect", InstanceID: request.InstanceID, PolicyGeneration: s.PolicyGeneration})
	if err != nil || instance.State != "running" || instance.ID != request.InstanceID {
		if s.connections[index] != nil && s.instanceIDs[index] == request.InstanceID {
			_ = s.connections[index].Close()
			s.connections[index] = nil
			s.instanceIDs[index] = ""
		}
		http.Error(w, "instance unavailable", 409)
		return
	}
	channel := filepath.Join(s.Channels, request.InstanceID, "adapter.sock")
	conn := s.connections[index]
	if conn != nil && s.instanceIDs[index] != request.InstanceID {
		_ = conn.Close()
		s.connections[index] = nil
		conn = nil
	}
	if conn == nil {
		conn, err = (&net.Dialer{Timeout: 3 * time.Second}).DialContext(ctx, "unix", channel)
		if err != nil {
			http.Error(w, "guest unavailable", 503)
			return
		}
		s.connections[index] = conn
		s.instanceIDs[index] = request.InstanceID
	}
	expectedUID := instance.GuestUID
	if expectedUID == 0 {
		expectedUID = s.SharedGuestUID
	}
	if err := authenticateGuestConnection(conn, expectedUID); err != nil {
		_ = conn.Close()
		s.connections[index] = nil
		http.Error(w, "guest identity denied", 503)
		return
	}
	healthy := false
	defer func() {
		if !healthy {
			_ = conn.Close()
			s.connections[index] = nil
		}
	}()
	aborted := make(chan struct{})
	stopAbort := context.AfterFunc(ctx, func() { _ = conn.Close(); close(aborted) })
	defer func() {
		if !stopAbort() {
			<-aborted
			healthy = false
		}
	}()
	deadline, _ := ctx.Deadline()
	if err = conn.SetDeadline(deadline); err != nil {
		http.Error(w, "guest deadline failed", 502)
		return
	}
	if err = ctx.Err(); err != nil {
		http.Error(w, "transfer cancelled", 503)
		return
	}
	if err = guestproto.Write(conn, request.Request); err != nil {
		http.Error(w, "guest write failed", 502)
		return
	}
	var response guestproto.Response
	if err = guestproto.Read(conn, &response); err != nil || response.Version != 1 || response.RequestID != request.Request.RequestID || len(response.Data) > guestproto.ChunkSize || len(response.Text) > 32<<10 || len(response.Error) > 128 || response.Offset < 0 || response.Size < 0 || response.Size > 512<<30 {
		http.Error(w, "invalid guest response", 502)
		return
	}
	if len(response.Data) > 0 {
		sum := sha256.Sum256(response.Data)
		if hex.EncodeToString(sum[:]) != response.SHA256 {
			http.Error(w, "guest checksum mismatch", 502)
			return
		}
	}
	healthy = true
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(response)
}

func authenticateGuestConnection(connection net.Conn, expectedUID uint32) error {
	if expectedUID == 0 || expectedUID > 1<<31-1 {
		return supervisor.ErrPolicy
	}
	unixConnection, ok := connection.(*net.UnixConn)
	if !ok || unixConnection == nil {
		return supervisor.ErrPolicy
	}
	uid, err := supervisor.PeerUID(unixConnection)
	if err != nil {
		return errors.Join(supervisor.ErrPolicy, err)
	}
	if uid != expectedUID {
		return supervisor.ErrPolicy
	}
	return nil
}
