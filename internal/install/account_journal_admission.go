package install

import (
	"os"
	"syscall"
)

// Account ownership intents must have one name and no executable special bits.
// A hard-linked journal cannot be treated as an exclusively owned installation
// record, even when its contents and ordinary permissions appear valid.
func accountJournalFileAdmitted(info os.FileInfo, owner int, maximum int64) bool {
	if info == nil || !info.Mode().IsRegular() || !owned(info, owner) || info.Mode().Perm() != 0600 || info.Mode()&(os.ModeSetuid|os.ModeSetgid|os.ModeSticky) != 0 || info.Size() < 0 || info.Size() > maximum {
		return false
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	return ok && stat.Nlink == 1
}

func (e *Engine) accountJournalPathUnchanged(name string, opened os.FileInfo, maximum int64) bool {
	current, err := e.journalRoot.Lstat(name)
	return err == nil && accountJournalFileAdmitted(current, e.owner, maximum) && os.SameFile(opened, current) && opened.Size() == current.Size() && opened.Mode() == current.Mode()
}
