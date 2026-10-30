package supervisor

import (
	"errors"
	"testing"
)

func TestQEMUOutputPipeMetadata(t *testing.T) {
	first := qemuOutputPipe{FD: 1, Target: "pipe:[1234]", Identity: qemuRegularIdentity{Device: 16, Inode: 1234, Links: 1, Mode: 0010000 | 0600}, Access: 1}
	second := first
	second.FD = 2
	if err := qualifyQEMUOutputPipes(first, second); err != nil {
		t.Fatal(err)
	}
	for _, mutation := range []string{"stdout-slot", "stderr-slot", "device", "inode", "zero-device", "zero-inode", "owner", "group", "mode", "type", "links", "readonly", "readwrite", "target", "target-inode", "leading-zero", "deleted"} {
		t.Run(mutation, func(t *testing.T) {
			out, errout := first, second
			switch mutation {
			case "stdout-slot":
				out.FD = 3
			case "stderr-slot":
				errout.FD = 3
			case "device":
				errout.Identity.Device++
			case "inode":
				errout.Identity.Inode++
			case "zero-device":
				out.Identity.Device = 0
				errout.Identity = out.Identity
			case "zero-inode":
				out.Identity.Inode = 0
				errout.Identity = out.Identity
			case "owner":
				out.Identity.UID = 200000
				errout.Identity = out.Identity
			case "group":
				out.Identity.GID = 108
				errout.Identity = out.Identity
			case "mode":
				out.Identity.Mode |= 0002
				errout.Identity = out.Identity
			case "type":
				out.Identity.Mode = 0140000 | 0600
				errout.Identity = out.Identity
			case "links":
				out.Identity.Links = 0
				errout.Identity = out.Identity
			case "readonly":
				out.Access = 0
			case "readwrite":
				errout.Access = 2
			case "target":
				errout.Target = "pipe:[5678]"
			case "target-inode":
				out.Target = "pipe:[5678]"
				errout.Target = out.Target
			case "leading-zero":
				out.Target = "pipe:[01234]"
				errout.Target = out.Target
			case "deleted":
				out.Target += " (deleted)"
				errout.Target = out.Target
			}
			if err := qualifyQEMUOutputPipes(out, errout); !errors.Is(err, ErrPolicy) {
				t.Fatal("unqualified pipe pair accepted", err)
			}
		})
	}
}
