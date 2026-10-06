package backup

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"unicode/utf8"
)

type recoveryManagementReceipt struct {
	Version int                      `json:"version"`
	Output  recoveryManagementOutput `json:"output"`
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
