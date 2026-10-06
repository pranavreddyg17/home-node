package backup

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"unicode/utf8"
)

type recoveryManagementReceipt struct {
	Version int                      `json:"version"`
	Output  recoveryManagementOutput `json:"output"`
}

// reconcileRecoveryManagementOutput reopens the private receipt and requires
// freshly validated output to match its recorded plan, bytes and checksum.
// Exclusive root ownership must continue through subsequent publication.
func reconcileRecoveryManagementOutput(ctx context.Context, root *os.Root) (receipt recoveryManagementReceipt, result error) {
	if err := ctx.Err(); err != nil {
		return receipt, err
	}
	if root == nil {
		return receipt, ErrManifest
	}
	directory, err := root.Stat(".")
	if err != nil || !directory.IsDir() || directory.Mode().Perm()&0077 != 0 {
		return receipt, ErrManifest
	}
	const name = "recovery-management.json"
	expected, err := root.Lstat(name)
	if err != nil {
		return receipt, err
	}
	if !expected.Mode().IsRegular() || expected.Mode().Perm() != 0600 || expected.Size() <= 0 || expected.Size() > 4096 {
		return receipt, ErrManifest
	}
	file, err := root.Open(name)
	if err != nil {
		return receipt, err
	}
	defer func() {
		result = errors.Join(result, file.Close())
		if result != nil {
			receipt = recoveryManagementReceipt{}
		}
	}()
	opened, err := file.Stat()
	if err != nil || !os.SameFile(expected, opened) {
		return receipt, ErrManifest
	}
	data, err := io.ReadAll(io.LimitReader(file, 4097))
	if err != nil {
		return receipt, err
	}
	receipt, err = decodeRecoveryManagementReceipt(data)
	if err != nil {
		return recoveryManagementReceipt{}, err
	}
	output, err := inspectReboundRecoveryManagement(ctx, root)
	if err != nil {
		return recoveryManagementReceipt{}, err
	}
	if output != receipt.Output {
		return recoveryManagementReceipt{}, ErrManifest
	}
	current, err := root.Lstat(name)
	if err != nil || !current.Mode().IsRegular() || !os.SameFile(opened, current) || current.Size() != int64(len(data)) || current.Mode().Perm() != 0600 {
		return recoveryManagementReceipt{}, ErrManifest
	}
	if err = ctx.Err(); err != nil {
		return recoveryManagementReceipt{}, err
	}
	return receipt, nil
}

func decodeRecoveryManagementReceipt(data []byte) (recoveryManagementReceipt, error) {
	if len(data) == 0 || len(data) > 4096 || !utf8.Valid(data) || uniqueJSON(json.NewDecoder(bytes.NewReader(data)), 0) != nil {
		return recoveryManagementReceipt{}, ErrManifest
	}
	fields, err := exactManifestObject(data, []string{"version", "output"}, "")
	if err != nil {
		return recoveryManagementReceipt{}, err
	}
	if _, err = exactManifestObject(fields["output"], []string{"planSha256", "sha256", "bytes"}, ""); err != nil {
		return recoveryManagementReceipt{}, err
	}
	var receipt recoveryManagementReceipt
	if json.Unmarshal(data, &receipt) != nil || receipt.Version != 1 || !repositoryPattern.MatchString(receipt.Output.PlanSHA256) || !repositoryPattern.MatchString(receipt.Output.SHA256) || receipt.Output.Bytes <= 0 || receipt.Output.Bytes > 256<<20 {
		return recoveryManagementReceipt{}, ErrManifest
	}
	return receipt, nil
}

// recordRecoveryManagementOutput persists newly inspected rebound bytes before
// database publication. It never overwrites an occupied receipt; reconciliation
// must reopen and verify that receipt rather than adopt a new output checksum.
func recordRecoveryManagementOutput(ctx context.Context, root *os.Root) error {
	output, err := inspectReboundRecoveryManagement(ctx, root)
	if err != nil {
		return err
	}
	data, err := json.Marshal(recoveryManagementReceipt{Version: 1, Output: output})
	if err != nil {
		return err
	}
	return createRecoveryJournal(ctx, root, "recovery-management.json", data)
}
