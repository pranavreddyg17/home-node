package runtimeclient

import (
	"context"
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"time"

	"github.com/pranavreddyg17/home-node/internal/disktransport"
	"github.com/pranavreddyg17/home-node/internal/supervisor"
)

type DiskClient struct{ socket string }

func NewDiskClient(socket string) *DiskClient { return &DiskClient{socket: socket} }

// WithMaintenanceDisk satisfies the backup builder's private disk bridge. It
// authenticates the root peer, validates the pinned descriptor and metadata,
// invokes the trusted copy, then closes the descriptor before acknowledgement.
// The Instance is a stopped disk summary, not runtime mutation authority.
func (c *DiskClient) WithMaintenanceDisk(ctx context.Context, token, id string, copyDisk func(context.Context, *os.File, supervisor.Instance) error) error {
	request := disktransport.Request{Version: 1, Token: token, InstanceID: id}
	if c == nil || copyDisk == nil || request.Validate() != nil || !filepath.IsAbs(c.socket) || filepath.Clean(c.socket) != c.socket {
		return disktransport.ErrPacket
	}
	deadline, cancel := context.WithTimeout(ctx, 2*time.Hour)
	defer cancel()
	connection, err := (&net.Dialer{Timeout: 3 * time.Second}).DialContext(deadline, "unixpacket", c.socket)
	if err != nil {
		return err
	}
	defer connection.Close()
	socket, ok := connection.(*net.UnixConn)
	if !ok {
		return disktransport.ErrPacket
	}
	peer, err := supervisor.PeerUID(socket)
	if err != nil || peer != 0 {
		return disktransport.ErrPacket
	}
	encoded, err := json.Marshal(request)
	if err != nil {
		return err
	}
	if err = disktransport.SendPacket(deadline, socket, encoded); err != nil {
		return err
	}
	return disktransport.CopySession(deadline, socket, func(ctx context.Context, data []byte, file *os.File) error {
		metadata, err := disktransport.DecodeMetadata(data)
		if err != nil || metadata.InstanceID != id {
			return disktransport.ErrPacket
		}
		if err = disktransport.AdmitDisk(file, metadata); err != nil {
			return err
		}
		instance := supervisor.Instance{ID: metadata.InstanceID, Workload: metadata.Workload, State: "stopped", Desired: "stopped", ImageSHA256: metadata.ImageSHA256, DataBytes: metadata.Bytes}
		return copyDisk(ctx, file, instance)
	})
}
