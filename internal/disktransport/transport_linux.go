//go:build linux

package disktransport

import (
	"context"
	"net"
	"os"
	"time"
	"unicode/utf8"

	"golang.org/x/sys/unix"
)

// Calls require exclusive connection ownership. Cancellation interrupts socket
// I/O, and received descriptors are atomically close-on-exec. Use unixpacket:
// stream sockets do not preserve the metadata/descriptor packet boundary.
func deadline(ctx context.Context, connection *net.UnixConn) (func(), error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	until := time.Now().Add(90 * time.Second)
	if requested, ok := ctx.Deadline(); ok {
		until = requested
		if maximum := time.Now().Add(2 * time.Hour); until.After(maximum) {
			until = maximum
		}
	}
	if err := connection.SetDeadline(until); err != nil {
		return nil, err
	}
	done := make(chan struct{})
	stop := context.AfterFunc(ctx, func() { _ = connection.SetDeadline(time.Now()); close(done) })
	return func() {
		if !stop() {
			<-done
		}
		_ = connection.SetDeadline(time.Time{})
	}, nil
}
func packetConnection(connection *net.UnixConn) bool {
	return connection != nil && connection.LocalAddr().Network() == "unixpacket"
}
func readonly(file *os.File) bool {
	if file == nil {
		return false
	}
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() {
		return false
	}
	flags, err := unix.FcntlInt(file.Fd(), unix.F_GETFL, 0)
	return err == nil && flags&unix.O_ACCMODE == unix.O_RDONLY
}
func SendFile(ctx context.Context, connection *net.UnixConn, data []byte, file *os.File) error {
	if !packetConnection(connection) || len(data) == 0 || len(data) > MaxPacket || !utf8.Valid(data) || !readonly(file) {
		return ErrPacket
	}
	cleanup, err := deadline(ctx, connection)
	if err != nil {
		return err
	}
	defer cleanup()
	rights := unix.UnixRights(int(file.Fd()))
	n, control, err := connection.WriteMsgUnix(data, rights, nil)
	if err != nil {
		return err
	}
	if n != len(data) || control != len(rights) {
		return ErrPacket
	}
	return nil
}
func ReceiveFile(ctx context.Context, connection *net.UnixConn) ([]byte, *os.File, error) {
	if !packetConnection(connection) {
		return nil, nil, ErrPacket
	}
	cleanup, err := deadline(ctx, connection)
	if err != nil {
		return nil, nil, err
	}
	defer cleanup()
	raw, err := connection.SyscallConn()
	if err != nil {
		return nil, nil, err
	}
	data, control := make([]byte, MaxPacket), make([]byte, 4096)
	var n, controlBytes, flags int
	var receiveErr error
	err = raw.Read(func(fd uintptr) bool {
		for {
			n, controlBytes, flags, _, receiveErr = unix.Recvmsg(int(fd), data, control, unix.MSG_CMSG_CLOEXEC)
			if receiveErr == unix.EINTR {
				continue
			}
			return receiveErr != unix.EAGAIN && receiveErr != unix.EWOULDBLOCK
		}
	})
	var descriptors []int
	messages, parseErr := unix.ParseSocketControlMessage(control[:controlBytes])
	valid := parseErr == nil
	for _, message := range messages {
		if message.Header.Level != unix.SOL_SOCKET || message.Header.Type != unix.SCM_RIGHTS {
			valid = false
			continue
		}
		received, rightsErr := unix.ParseUnixRights(&message)
		if rightsErr != nil {
			valid = false
		}
		descriptors = append(descriptors, received...)
	}
	accepted := false
	defer func() {
		if !accepted {
			for _, fd := range descriptors {
				_ = unix.Close(fd)
			}
		}
	}()
	if err != nil {
		return nil, nil, err
	}
	if receiveErr != nil {
		return nil, nil, receiveErr
	}
	if !valid || flags&(unix.MSG_TRUNC|unix.MSG_CTRUNC) != 0 || n == 0 || !utf8.Valid(data[:n]) || len(descriptors) != 1 {
		return nil, nil, ErrPacket
	}
	descriptorFlags, err := unix.FcntlInt(uintptr(descriptors[0]), unix.F_GETFD, 0)
	if err != nil || descriptorFlags&unix.FD_CLOEXEC == 0 {
		return nil, nil, ErrPacket
	}
	file := os.NewFile(uintptr(descriptors[0]), "maintenance-disk")
	if !readonly(file) {
		if file != nil {
			_ = file.Close()
			descriptors = nil
		}
		return nil, nil, ErrPacket
	}
	accepted = true
	return data[:n], file, nil
}
