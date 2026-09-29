package install

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"strconv"
	"syscall"
)

func errorsJoin(values ...error) error { return errors.Join(values...) }

func (e *Engine) parents(name string) error {
	for parent := path.Dir(name); parent != "."; parent = path.Dir(parent) {
		info, err := e.host.Lstat(parent)
		if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 || !owned(info, e.owner) || info.Mode().Perm()&0022 != 0 {
			return ErrConflict
		}
	}
	return nil
}
func (e *Engine) matches(r record) error {
	if err := e.parents(r.Path); err != nil {
		return err
	}
	file, err := e.host.OpenFile(r.Path, os.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
	if err != nil {
		return err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return err
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok || int(stat.Uid) != r.UID || int(stat.Gid) != r.GID || info.Mode().Perm() != mode(r) || info.IsDir() != r.Directory {
		return ErrConflict
	}
	if r.Directory {
		return nil
	}
	if !info.Mode().IsRegular() {
		return ErrConflict
	}
	data, err := io.ReadAll(io.LimitReader(file, maxFileBytes+1))
	if err != nil || len(data) > maxFileBytes || digest(data) != r.SHA256 {
		return ErrConflict
	}
	return nil
}
func (e *Engine) stage(j journal, index int) string {
	return path.Join(path.Dir(j.Items[index].Path), ".homenode-"+j.ID+"-"+strconv.Itoa(index)+".stage")
}
func (e *Engine) removeStage(j journal, index int) error {
	name := e.stage(j, index)
	info, err := e.host.Lstat(name)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() || !owned(info, e.owner) || info.Mode().Perm()&0022 != 0 {
		return ErrConflict
	}
	return e.host.Remove(name)
}

func (e *Engine) Apply(ctx context.Context, plan Plan) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return err
	}
	records, hash, err := planRecords(plan, e.owner)
	if err != nil {
		return err
	}
	j, err := e.load()
	if os.IsNotExist(err) {
		j = journal{Version: 1, Digest: hash, Phase: "installing", Items: records}
		j.ID, err = newID()
		if err != nil {
			return err
		}
		// Refuse occupied configuration before recording any ownership intent.
		for index, r := range records {
			if _, err = e.host.Lstat(r.Path); err == nil {
				if !r.Directory || e.matches(r) != nil {
					return fmt.Errorf("%s: %w", r.Path, ErrConflict)
				}
				j.Items[index].State = "existing"
			} else if !os.IsNotExist(err) {
				return err
			}
		}
		if err = e.save(j); err != nil {
			return err
		}
	} else if err != nil {
		return err
	}
	if j.Digest != hash || (j.Phase != "installing" && j.Phase != "installed") {
		return ErrConflict
	}
	for index, r := range j.Items {
		if err = ctx.Err(); err != nil {
			return err
		}
		if r.State == "created" || r.State == "existing" {
			if err = e.matches(r); err != nil {
				return fmt.Errorf("%s: %w", r.Path, ErrConflict)
			}
			continue
		}
		if r.State != "pending" {
			return ErrConflict
		}
		if err = e.parents(r.Path); err != nil {
			return err
		}
		if r.Directory {
			// A pending directory may have been created just before a crash. Only
			// accept its expected final attributes or private initial root attributes.
			info, probe := e.host.Lstat(r.Path)
			if os.IsNotExist(probe) {
				err = e.host.Mkdir(r.Path, 0700)
			} else if probe != nil {
				err = probe
			} else if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 || (e.matches(r) != nil && (!owned(info, e.owner) || info.Mode().Perm() != 0700)) {
				err = ErrConflict
			}
			if err != nil {
				return err
			}
			file, err := e.host.OpenFile(r.Path, os.O_RDONLY|syscall.O_DIRECTORY|syscall.O_NOFOLLOW, 0)
			if err != nil {
				return err
			}
			err = file.Chown(r.UID, r.GID)
			if err == nil {
				err = file.Chmod(mode(r))
			}
			if err == nil {
				err = file.Sync()
			}
			err = errorsJoin(err, file.Close())
			if err != nil {
				return err
			}
			if err = syncDirectory(e.host, path.Dir(r.Path)); err != nil {
				return err
			}
		} else {
			if _, probe := e.host.Lstat(r.Path); probe == nil {
				if err = e.matches(r); err != nil {
					return ErrConflict
				}
			} else if !os.IsNotExist(probe) {
				return probe
			} else {
				if err = e.removeStage(j, index); err != nil {
					return err
				}
				stage := e.stage(j, index)
				file, err := e.host.OpenFile(stage, os.O_WRONLY|os.O_CREATE|os.O_EXCL|syscall.O_NOFOLLOW, 0600)
				if err != nil {
					return err
				}
				if e.checkpoint != nil {
					if failure := e.checkpoint("staged", r.Path); failure != nil {
						return errorsJoin(failure, file.Close())
					}
				}
				_, err = file.Write(plan.Items[index].Data)
				if err == nil {
					err = file.Chown(r.UID, r.GID)
				}
				if err == nil {
					err = file.Chmod(mode(r))
				}
				if err == nil {
					err = file.Sync()
				}
				err = errorsJoin(err, file.Close())
				if err != nil {
					return err
				}
				// Link is atomic and cannot overwrite a foreign destination.
				if err = e.host.Link(stage, r.Path); err != nil {
					return err
				}
				if err = syncDirectory(e.host, path.Dir(r.Path)); err != nil {
					return err
				}
			}
			if err = e.removeStage(j, index); err != nil {
				return err
			}
			if err = syncDirectory(e.host, path.Dir(r.Path)); err != nil {
				return err
			}
		}
		if e.checkpoint != nil {
			if err = e.checkpoint("published", r.Path); err != nil {
				return err
			}
		}
		j.Items[index].State = "created"
		if err = e.save(j); err != nil {
			return err
		}
	}
	j.Phase = "installed"
	return e.save(j)
}

// Rollback only removes unchanged owned files and empty created directories.
// The caller must stop services first. Owner content is never traversed/deleted.
func (e *Engine) Rollback(ctx context.Context) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return err
	}
	j, err := e.load()
	if err != nil {
		return err
	}
	if j.Phase == "rolled-back" {
		return nil
	}
	j.Phase = "rolling-back"
	if err = e.save(j); err != nil {
		return err
	}
	for index := len(j.Items) - 1; index >= 0; index-- {
		if err = ctx.Err(); err != nil {
			return err
		}
		r := j.Items[index]
		if r.State == "removed" || r.State == "existing" {
			continue
		}
		if err = e.parents(r.Path); err != nil {
			return err
		}
		if _, err = e.host.Lstat(r.Path); err == nil {
			if err = e.matches(r); err != nil {
				info, probe := e.host.Lstat(r.Path)
				if r.State != "pending" || !r.Directory || probe != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 || !owned(info, e.owner) || info.Mode().Perm() != 0700 {
					return fmt.Errorf("%s changed; %w", r.Path, ErrConflict)
				}
			}
			if err = e.host.Remove(r.Path); err != nil {
				return fmt.Errorf("%s is occupied or cannot be removed: %w", r.Path, err)
			}
		} else if !os.IsNotExist(err) {
			return err
		}
		if !r.Directory {
			if err = e.removeStage(j, index); err != nil {
				return err
			}
		}
		if err = syncDirectory(e.host, path.Dir(r.Path)); err != nil {
			return err
		}
		if e.checkpoint != nil {
			if err = e.checkpoint("removed", r.Path); err != nil {
				return err
			}
		}
		j.Items[index].State = "removed"
		if err = e.save(j); err != nil {
			return err
		}
	}
	j.Phase = "rolled-back"
	return e.save(j)
}
