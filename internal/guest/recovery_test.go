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

func TestTaskRecoveryRejectsUnsafeJournalsBeforeRewriting(t *testing.T) {
	for _, which := range []string{"oversize", "symlink", "fifo", "unknown", "duplicate", "trailing", "state", "profile"} {
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
			case "unknown":
				data = []byte(`{"state":"running","command":"shell"}`)
			case "duplicate":
				data = []byte(`{"state":"running","state":"succeeded"}`)
			case "trailing":
				data = append(append([]byte{}, good...), []byte(` {}`)...)
			case "profile":
				data, _ = json.Marshal(task{State: "running", PromptHash: promptDigest(guestproto.Request{})})
			}
			if which != "symlink" && which != "fifo" {
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
