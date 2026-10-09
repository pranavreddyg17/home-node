package install

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"reflect"
	"strings"
	"syscall"
)

// Retain configuration intent while independently matching proposal and current
// installation authority. Source/destination bytes are re-derived, never adopted.
func (e *Engine) withGuestStorageConfigurationIntent(ctx context.Context, current journal, plan GuestStorageProvisioningPlan, use func(guestStorageConfigurationIntent, func() error) error) (result error) {
	if err := ctx.Err(); err != nil {
		return err
	}
	if use == nil {
		return ErrPlan
	}
	if _, err := canonicalGuestStoragePlan(ctx, plan); err != nil {
		return err
	}
	const name = "guest-storage-configuration-intent.json"
	const maximum = 262144
	file, err := e.journalRoot.OpenFile(name, os.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
	if err != nil {
		return err
	}
	defer func() { result = errors.Join(result, file.Close()) }()
	info, err := file.Stat()
	if err != nil || !accountJournalFileAdmitted(info, e.owner, maximum) {
		return ErrConflict
	}
	data, err := io.ReadAll(io.LimitReader(file, maximum+1))
	if err != nil {
		return err
	}
	var intent guestStorageConfigurationIntent
	if len(data) > maximum || json.Unmarshal(data, &intent) != nil || intent.Version != 1 || !reflect.DeepEqual(intent.Plan, plan) || len(intent.Original.ID) != 32 || strings.Trim(intent.Original.ID, "0123456789abcdef") != "" || len(intent.Original.Items) == 0 || len(intent.Original.Items) > maxInstallationItems {
		return ErrConflict
	}
	proposal, err := json.Marshal(guestStorageIntent{Version: 1, Plan: plan})
	if err != nil {
		return err
	}
	desired, policy, environment, err := planGuestStorageConfiguration(ctx, intent.Original, intent.SourcePolicy, intent.SourceEnvironment, plan)
	if err != nil {
		return err
	}
	if intent.StorageIntentSHA256 != digest(proposal) || !reflect.DeepEqual(desired, intent.Desired) || !bytes.Equal(policy, intent.Policy) || !bytes.Equal(environment, intent.Environment) || (!reflect.DeepEqual(current, intent.Original) && !reflect.DeepEqual(current, intent.Desired)) {
		return ErrConflict
	}
	canonical, err := json.Marshal(intent)
	if err != nil {
		return err
	}
	if !bytes.Equal(canonical, data) || !e.accountJournalPathUnchanged(name, info, maximum) {
		return ErrConflict
	}
	check := func() error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if _, err := file.Seek(0, io.SeekStart); err != nil {
			return err
		}
		contents, err := io.ReadAll(io.LimitReader(file, maximum+1))
		if err != nil {
			return err
		}
		observed, err := file.Stat()
		if err != nil || !accountJournalFileAdmitted(observed, e.owner, maximum) || !os.SameFile(info, observed) || observed.Mode() != info.Mode() || observed.Size() != info.Size() || !bytes.Equal(contents, data) || !e.accountJournalPathUnchanged(name, info, maximum) {
			return ErrConflict
		}
		return ctx.Err()
	}
	if err := check(); err != nil {
		return err
	}
	if err := use(intent, check); err != nil {
		return err
	}
	return check()
}
