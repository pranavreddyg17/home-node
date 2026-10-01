package supervisor

import (
	"context"
	"encoding/json"
	"net"
	"os"
	"time"

	"github.com/pranavreddyg17/home-node/internal/disktransport"
)

// ServeMaintenanceDisk owns one authenticated backup connection. The root disk
// guard remains held until the copy session acknowledges completion or fails.
// Production socket installation and accept-loop budgets are configured outside
// this per-connection handler. No request can select a host filesystem path.
func (m *Manager) ServeMaintenanceDisk(ctx context.Context, connection *net.UnixConn, backupUID uint32) error {
	if connection == nil {
		return ErrPolicy
	}
	defer connection.Close()
	if backupUID == 0 || backupUID == m.Policy.ControllerUID || backupUID == m.Policy.TransferUID {
		return ErrPolicy
	}
	peer, err := PeerUID(connection)
	if err != nil || peer != backupUID {
		return ErrPolicy
	}
	deadline, cancel := context.WithTimeout(ctx, 2*time.Hour)
	defer cancel()
	// Admission packets have a short deadline; copying gets the bounded session.
	admission, cancelAdmission := context.WithTimeout(deadline, 5*time.Second)
	data, err := disktransport.ReceivePacket(admission, connection)
	cancelAdmission()
	if err != nil {
		return err
	}
	request, err := disktransport.DecodeRequest(data)
	if err != nil {
		return err
	}
	return m.WithMaintenanceDisk(deadline, request.Token, request.InstanceID, func(ctx context.Context, file *os.File, instance Instance) error {
		metadata := disktransport.Metadata{Version: 1, InstanceID: instance.ID, Workload: instance.Workload, Bytes: instance.DataBytes, ImageSHA256: instance.ImageSHA256, DataSchema: 1, Protocol: 1}
		if err := disktransport.AdmitDisk(file, metadata); err != nil {
			return err
		}
		encoded, err := json.Marshal(metadata)
		if err != nil {
			return err
		}
		return disktransport.SendAndWait(ctx, connection, encoded, file)
	})
}
