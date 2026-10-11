package networkcheck

import (
	"crypto/tls"
	"fmt"
	"io"
	"os"
	"syscall"
	"time"
)

// Read protected files by descriptor and reject final symlinks, unsafe modes,
// oversized PEMs and path replacement between observation and opening.
func protectedPEM(path string, private bool) ([]byte, error) {
	before, err := os.Lstat(path)
	if err != nil || !before.Mode().IsRegular() {
		return nil, fmt.Errorf("TLS identity file is unavailable or not regular: %w", ErrIdentity)
	}
	file, err := os.OpenFile(path, os.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
	if err != nil {
		return nil, ErrIdentity
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || !before.Mode().IsRegular() || !info.Mode().IsRegular() || !os.SameFile(before, info) || info.Mode().Perm()&0022 != 0 {
		return nil, fmt.Errorf("TLS files must be regular, root owned and protected from modification: %w", ErrIdentity)
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok || stat.Uid != 0 || stat.Nlink != 1 || info.Mode()&(os.ModeSetuid|os.ModeSetgid|os.ModeSticky) != 0 || info.Mode().Perm()&0111 != 0 || (private && info.Mode().Perm() != 0600 && info.Mode().Perm() != 0640) {
		return nil, fmt.Errorf("TLS key needs root ownership and no access for other users: %w", ErrIdentity)
	}
	data, err := io.ReadAll(io.LimitReader(file, (64<<10)+1))
	if err != nil || len(data) == 0 || len(data) > 64<<10 || int64(len(data)) != info.Size() {
		return nil, ErrIdentity
	}
	after, err := file.Stat()
	if err != nil || !os.SameFile(info, after) || info.Size() != after.Size() || info.Mode() != after.Mode() || !info.ModTime().Equal(after.ModTime()) {
		return nil, ErrIdentity
	}
	afterStat, ok := after.Sys().(*syscall.Stat_t)
	named, err := os.Lstat(path)
	if !ok || afterStat.Uid != stat.Uid || afterStat.Gid != stat.Gid || afterStat.Nlink != 1 || err != nil || !os.SameFile(after, named) || named.Mode() != after.Mode() || named.Size() != after.Size() {
		return nil, ErrIdentity
	}
	return data, nil
}

func Inspect(c Config, certPath, keyPath string) (tls.Certificate, error) {
	if err := inspectTailnet(c); err != nil {
		return tls.Certificate{}, err
	}
	cert, err := protectedPEM(certPath, false)
	if err != nil {
		return tls.Certificate{}, err
	}
	key, err := protectedPEM(keyPath, true)
	if err != nil {
		return tls.Certificate{}, err
	}
	addresses, err := LocalAddresses()
	if err != nil {
		return tls.Certificate{}, fmt.Errorf("cannot inspect local network interfaces: %w", ErrIdentity)
	}
	return Validate(c, addresses, cert, key, nil, time.Now())
}
