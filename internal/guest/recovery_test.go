package guest

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pranavreddyg17/home-node/internal/guestproto"
	"github.com/pranavreddyg17/home-node/internal/state"
	"golang.org/x/sys/unix"
)

func TestTaskRecoveryRefusesUnsafeTemporaryEntriesBeforeTruncation(t *testing.T) {
	for _, kind := range []string{"symlink", "hardlink", "fifo", "writable", "oversize"} {
		t.Run(kind, func(t *testing.T) {
			directory := privateDataDir(t)
			id := state.Random()
			journal, _ := json.Marshal(task{State: "running", InputID: state.Random(), Preset: "mp4-720p", PromptHash: promptDigest(guestproto.Request{})})
			path := filepath.Join(directory, id+".task.json")
			if err := os.WriteFile(path, journal, 0600); err != nil {
				t.Fatal(err)
			}
			otherPath := filepath.Join(directory, state.Random()+".task.json")
			if err := os.WriteFile(otherPath, journal, 0600); err != nil {
				t.Fatal(err)
			}
			target := filepath.Join(directory, state.Random()+".blob")
			original := []byte("preserved object bytes")
			if err := os.WriteFile(target, original, 0600); err != nil {
				t.Fatal(err)
			}
			temporary := filepath.Join(directory, id+".task.tmp")
			var err error
			switch kind {
			case "symlink":
				err = os.Symlink(target, temporary)
			case "hardlink":
				err = os.Link(target, temporary)
			case "fifo":
				err = unix.Mkfifo(temporary, 0600)
			case "writable":
				err = os.WriteFile(temporary, original, 0644)
			case "oversize":
				err = os.WriteFile(temporary, bytes.Repeat([]byte("x"), maxTaskJournalBytes+1), 0600)
			}
			if err != nil {
				t.Fatal(err)
			}
			before, err := os.Lstat(temporary)
			if err != nil {
				t.Fatal(err)
			}
			if a, err := New(directory, "video", 8<<30); err == nil {
				a.Close()
				t.Fatal("unsafe temporary task journal admitted")
			}
			for name, expected := range map[string][]byte{target: original, path: journal, otherPath: journal} {
				actual, err := os.ReadFile(name)
				if err != nil || !bytes.Equal(actual, expected) {
					t.Fatal("recovery refusal changed evidence", name, err)
				}
			}
			after, err := os.Lstat(temporary)
			if err != nil || !os.SameFile(before, after) || before.Mode() != after.Mode() || before.Size() != after.Size() {
				t.Fatal("recovery refusal replaced temporary entry", err)
			}
		})
	}
}

func TestTaskRecoveryRejectsUnsafeJournalsBeforeRewriting(t *testing.T) {
	for _, which := range []string{"oversize", "symlink", "hardlink", "fifo", "unknown", "duplicate", "trailing", "state", "profile"} {
		t.Run(which, func(t *testing.T) {
			dir := privateDataDir(t)
			id := state.Random()
			good, _ := json.Marshal(task{State: "running", InputID: state.Random(), Preset: "mp4-720p", PromptHash: promptDigest(guestproto.Request{})})
			goodPath := filepath.Join(dir, id+".task.json")
			if err := os.WriteFile(goodPath, good, 0600); err != nil {
				t.Fatal(err)
			}
			bad := filepath.Join(dir, state.Random()+".task.json")
			data := []byte(`{"state":"broken"}`)
			switch which {
			case "oversize":
				data = bytes.Repeat([]byte("x"), maxTaskJournalBytes+1)
			case "symlink":
				if err := os.Symlink(goodPath, bad); err != nil {
					t.Fatal(err)
				}
			case "fifo":
				if err := unix.Mkfifo(bad, 0600); err != nil {
					t.Fatal(err)
				}
			case "hardlink":
				if err := os.Link(goodPath, bad); err != nil {
					t.Fatal(err)
				}
			case "unknown":
				data = []byte(`{"state":"running","command":"shell"}`)
			case "duplicate":
				data = []byte(`{"state":"running","state":"succeeded"}`)
			case "trailing":
				data = append(append([]byte{}, good...), []byte(` {}`)...)
			case "profile":
				data, _ = json.Marshal(task{State: "running", PromptHash: promptDigest(guestproto.Request{})})
			}
			if which != "symlink" && which != "hardlink" && which != "fifo" {
				if err := os.WriteFile(bad, data, 0600); err != nil {
					t.Fatal(err)
				}
			}
			if a, err := New(dir, "video", 8<<30); err == nil {
				a.Close()
				t.Fatal("unsafe journal accepted")
			}
			after, err := os.ReadFile(goodPath)
			if err != nil || !bytes.Equal(good, after) {
				t.Fatal("valid intent rewritten before validation", err)
			}
		})
	}
}
func TestEscapedMaximumAITextRemainsRecoverable(t *testing.T) {
	dir := privateDataDir(t)
	a, err := New(dir, "ai", 1<<30)
	if err != nil {
		t.Fatal(err)
	}
	id := state.Random()
	record := task{State: "succeeded", PromptHash: promptDigest(guestproto.Request{}), Text: strings.Repeat("\x01", 32<<10)}
	if err = a.save(id, &record); err != nil {
		t.Fatal(err)
	}
	if err = a.Close(); err != nil {
		t.Fatal(err)
	}
	a, err = New(dir, "ai", 1<<30)
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	if a.tasks[id].Text != record.Text {
		t.Fatal("escaped output lost")
	}
}
