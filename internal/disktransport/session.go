package disktransport

import (
	"bytes"
	"context"
	"errors"
	"net"
	"os"
)

const copyAcknowledgement = `{"version":1,"copied":true}`

// SendAndWait owns the connection until the copy acknowledges completion or
// fails. Invoke it inside the supervisor's owned disk callback so the runtime
// guard remains held throughout copying. It does not close the caller's file.
func SendAndWait(ctx context.Context, connection *net.UnixConn, metadata []byte, file *os.File) error {
	if connection == nil {
		return ErrPacket
	}
	defer connection.Close()
	if err := SendFile(ctx, connection, metadata, file); err != nil {
		return err
	}
	acknowledgement, err := ReceivePacket(ctx, connection)
	if err != nil {
		return err
	}
	if !bytes.Equal(acknowledgement, []byte(copyAcknowledgement)) {
		return ErrPacket
	}
	return nil
}

// CopySession owns the connection and received descriptor. The trusted callback
// must validate metadata and complete its copy before returning. The descriptor
// is closed before acknowledgement, including error/cancellation paths; failed
// copies never acknowledge success. Callers must authenticate the root peer.
func CopySession(ctx context.Context, connection *net.UnixConn, copyDisk func(context.Context, []byte, *os.File) error) error {
	if connection == nil || copyDisk == nil {
		return ErrPacket
	}
	defer connection.Close()
	metadata, file, err := ReceiveFile(ctx, connection)
	if err != nil {
		return err
	}
	// Also close on callback panic; no acknowledgement is emitted in that path.
	defer file.Close()
	err = copyDisk(ctx, metadata, file)
	err = errors.Join(err, file.Close(), ctx.Err())
	if err != nil {
		return err
	}
	return SendPacket(ctx, connection, []byte(copyAcknowledgement))
}
