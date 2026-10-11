package install

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"runtime"
	"syscall"
)

type gatewayRemovalIntent struct {
	Version int                       `json:"version"`
	Plan    GatewayAccountRemovalPlan `json:"plan"`
}

// PrepareGatewayAccountRemoval persists intent only. It does not stop services,
// remove identities, release activation exclusion or delete application data.
func (e *Engine) PrepareGatewayAccountRemoval(ctx context.Context) (GatewayAccountRemovalPlan, error) {
	if runtime.GOOS != "linux" || os.Geteuid() != 0 || e.host.Name() != "/" {
		return GatewayAccountRemovalPlan{}, ErrAccounts
	}
	return e.prepareGatewayAccountRemoval(ctx, nativeGatewayProvisioner{})
}
func (e *Engine) prepareGatewayAccountRemoval(ctx context.Context, b gatewayProvisionBackend) (GatewayAccountRemovalPlan, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	var empty GatewayAccountRemovalPlan
	plan, err := e.planGatewayAccountRemovalLocked(ctx, b)
	if err != nil {
		return empty, err
	}
	expected, err := json.Marshal(gatewayRemovalIntent{Version: 1, Plan: plan})
	if err != nil {
		return empty, err
	}
	file, err := e.journalRoot.OpenFile("gateway-account-removal.json", os.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
	if err == nil {
		info, statErr := file.Stat()
		if statErr != nil || !accountJournalFileAdmitted(info, e.owner, 16384) {
			file.Close()
			return empty, ErrConflict
		}
		data, readErr := io.ReadAll(io.LimitReader(file, 16385))
		closeErr := file.Close()
		if readErr != nil || closeErr != nil || len(data) > 16384 || !bytes.Equal(data, expected) || !e.accountJournalPathUnchanged("gateway-account-removal.json", info, 16384) {
			return empty, ErrConflict
		}
		return plan, nil
	}
	if !os.IsNotExist(err) {
		return empty, err
	}
	if err := ctx.Err(); err != nil {
		return empty, err
	}
	if err := e.saveJournalBytes("gateway-account-removal", expected); err != nil {
		return empty, err
	}
	return plan, nil
}
