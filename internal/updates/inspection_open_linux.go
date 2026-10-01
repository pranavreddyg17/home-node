//go:build linux

package updates

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"

	"golang.org/x/sys/unix"
)

// InspectionStage retains the exclusive staging lock and read-only package
// descriptor until the protected caller finishes worker execution and result
// verification. Closing it leaves durable state intact for explicit recovery.
type InspectionStage struct {
	Package   *os.File
	directory *os.File
	identity  InspectionIdentity
}

func (s *InspectionStage) Close() error { return errors.Join(s.Package.Close(), s.directory.Close()) }

// Environment binds service inputs to the identity admitted under this lock.
func (s *InspectionStage) Environment() ([]byte, error) {
	if s == nil || s.Package == nil || s.directory == nil {
		return nil, ErrInspectionResult
	}
	return InspectionEnvironment(s.identity)
}

// VerifyResult checks bounded worker output against retained operation identity
// and rehashes the pinned package after worker completion. The caller must first
// authenticate successful isolated execution; this method grants no installation
// authority and does not treat a JSON response as execution evidence.
func (s *InspectionStage) VerifyResult(ctx context.Context, output []byte) (InspectionResult, error) {
	if s == nil || s.Package == nil || s.directory == nil {
		return InspectionResult{}, ErrInspectionResult
	}
	result, err := ValidateInspectionResult(output, s.identity)
	if err != nil {
		return InspectionResult{}, err
	}
	digest, length, err := PackageIdentity(ctx, s.Package)
	if err != nil || digest != s.identity.PackageSHA256 || length != s.identity.PackageLength {
		return InspectionResult{}, errors.Join(ErrInspectionResult, err)
	}
	return result, nil
}

// OpenInspectionStage verifies ready publication against independently retained
// operation identity. It does not infer identity from untrusted disk records.
func OpenInspectionStage(ctx context.Context, root *os.Root, expected InspectionIdentity) (*InspectionStage, error) {
	if os.Geteuid() != 0 {
		return nil, ErrInspectionResult
	}
	return openInspectionStageOwned(ctx, root, expected)
}

func openInspectionStageOwned(ctx context.Context, root *os.Root, expected InspectionIdentity) (_ *InspectionStage, resultErr error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if root == nil || !validInspectionIdentity(expected) {
		return nil, ErrInspectionResult
	}
	directory, err := root.Open(".")
	if err != nil {
		return nil, err
	}
	keep := false
	defer func() {
		if !keep {
			resultErr = errors.Join(resultErr, directory.Close())
		}
	}()
	var stat unix.Stat_t
	if unix.Fstat(int(directory.Fd()), &stat) != nil || stat.Mode != unix.S_IFDIR|0700 || stat.Uid != uint32(os.Geteuid()) || stat.Gid != uint32(os.Getegid()) {
		return nil, ErrInspectionResult
	}
	if err := unix.Flock(int(directory.Fd()), unix.LOCK_EX|unix.LOCK_NB); err != nil {
		return nil, err
	}
	entries, err := directory.ReadDir(4)
	if err != nil && !errors.Is(err, io.EOF) || len(entries) != 3 {
		return nil, ErrInspectionResult
	}
	for _, entry := range entries {
		if entry.Name() != "intent" && entry.Name() != "ready" && entry.Name() != "package.deb" {
			return nil, ErrInspectionResult
		}
	}
	canonical, err := json.Marshal(struct {
		Schema      int    `json:"schema"`
		OperationID string `json:"operationId"`
		Release     string `json:"release"`
		SHA256      string `json:"sha256"`
		Length      int64  `json:"length"`
	}{1, expected.OperationID, expected.Release, expected.PackageSHA256, expected.PackageLength})
	if err != nil {
		return nil, err
	}
	for _, name := range []string{"intent", "ready"} {
		file, err := openInspectionFile(root, name, 0600)
		if err != nil {
			return nil, err
		}
		data, readErr := io.ReadAll(io.LimitReader(file, 2049))
		closeErr := file.Close()
		if readErr != nil || closeErr != nil || !bytes.Equal(data, canonical) {
			return nil, errors.Join(ErrInspectionResult, readErr, closeErr)
		}
	}
	file, err := openInspectionFile(root, "package.deb", 0400)
	if err != nil {
		return nil, err
	}
	digest, length, err := PackageIdentity(ctx, file)
	if err != nil || digest != expected.PackageSHA256 || length != expected.PackageLength {
		return nil, errors.Join(ErrInspectionResult, err, file.Close())
	}
	keep = true
	return &InspectionStage{Package: file, directory: directory, identity: expected}, nil
}

func openInspectionFile(root *os.Root, name string, mode uint32) (*os.File, error) {
	file, err := root.OpenFile(name, os.O_RDONLY|unix.O_NOFOLLOW|unix.O_NONBLOCK, 0)
	if err != nil {
		return nil, err
	}
	var stat unix.Stat_t
	if unix.Fstat(int(file.Fd()), &stat) != nil || stat.Mode != unix.S_IFREG|mode || stat.Uid != uint32(os.Geteuid()) || stat.Gid != uint32(os.Getegid()) || stat.Nlink != 1 {
		return nil, errors.Join(ErrInspectionResult, file.Close())
	}
	return file, nil
}
