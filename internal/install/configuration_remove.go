package install

import (
	"context"
	"encoding/json"
	"os"
	"path"
	"reflect"
)

// removeConfigurationLocked consumes canonical intent under a retained guard.
// Its caller must hold installation authority and accommodate the exact ordered
// removal transition when observing loaded units. No public API uses it yet.
func (e *Engine) removeConfigurationLocked(ctx context.Context, guard func(context.Context) error) error {
	if guard == nil {
		return ErrPlan
	}
	installed, err := e.load()
	if err != nil {
		return err
	}
	intent, err := e.loadConfigurationRemovalIntent(installed)
	if err != nil {
		return err
	}
	records := map[string]record{}
	for _, item := range installed.Items {
		records[item.Path] = item
	}
	observe := func() error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := guard(ctx); err != nil {
			return err
		}
		current, err := e.load()
		if err != nil || !reflect.DeepEqual(current, installed) {
			return ErrConflict
		}
		for _, retained := range intent.Plan.RetainedPaths {
			if err := e.matches(records[retained]); err != nil {
				return err
			}
		}
		for index, file := range intent.Plan.Files {
			if err := ctx.Err(); err != nil {
				return err
			}
			if err := e.parents(file.Path); err != nil {
				return err
			}
			_, err := e.host.Lstat(file.Path)
			if os.IsNotExist(err) {
				// Only the first unacknowledged unlink may be absent. Missing future
				// configuration is a conflict, not an invitation to claim its removal.
				if index > intent.Completed {
					return ErrConflict
				}
				continue
			}
			if err != nil {
				return err
			}
			if index < intent.Completed || intent.Ready {
				return ErrConflict
			}
			if err := e.matches(records[file.Path]); err != nil {
				return err
			}
		}
		return guard(ctx)
	}
	persist := func() error {
		data, err := json.Marshal(intent)
		if err != nil {
			return err
		}
		return e.saveJournalBytes("configuration-removal", data)
	}
	for index, file := range intent.Plan.Files {
		if err := observe(); err != nil {
			return err
		}
		if index < intent.Completed {
			continue
		}
		_, err := e.host.Lstat(file.Path)
		if err == nil {
			if err := e.matches(records[file.Path]); err != nil {
				return err
			}
			if err := guard(ctx); err != nil {
				return err
			}
			if err := ctx.Err(); err != nil {
				return err
			}
			if err := e.host.Remove(file.Path); err != nil {
				return err
			}
		} else if !os.IsNotExist(err) {
			return err
		}
		// Also sync lost acknowledgements before checkpointing their durable absence.
		if err := syncDirectory(e.host, path.Dir(file.Path)); err != nil {
			return err
		}
		if err := observe(); err != nil {
			return err
		}
		if e.checkpoint != nil {
			if err := e.checkpoint("configuration-removed", file.Path); err != nil {
				return err
			}
		}
		intent.Completed = index + 1
		if err := persist(); err != nil {
			return err
		}
	}
	if err := observe(); err != nil {
		return err
	}
	if !intent.Ready {
		intent.Ready = true
		return persist()
	}
	return nil
}
