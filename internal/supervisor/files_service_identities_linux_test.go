//go:build linux

package supervisor

import (
	"context"
	"os"
	"os/user"
	"strconv"
	"testing"
	"time"
)

// Only the explicitly enabled disposable KVM fixture may create local accounts.
// Never adopt existing identities or delete accounts that this fixture did not create.
func nativeFilesServiceIdentities(t *testing.T, ctx context.Context) (uint32, uint32, uint32) {
	t.Helper()
	if os.Geteuid() != 0 || os.Getenv("HOMENODE_KVM_DOMAIN_INTEGRATION") != "1" || os.Getenv("HOMENODE_FILES_MANAGER_INTEGRATION") != "1" {
		t.Fatal("explicit disposable Files identity fixture required")
	}
	for _, name := range []string{"homenode", "homenode-transfer", "homenode-runtime"} {
		if _, err := user.Lookup(name); err == nil {
			t.Fatal("refusing existing fixture user", name)
		} else if _, ok := err.(user.UnknownUserError); !ok {
			t.Fatal("cannot qualify absent fixture user", name, err)
		}
		if _, err := user.LookupGroup(name); err == nil {
			t.Fatal("refusing existing fixture group", name)
		} else if _, ok := err.(user.UnknownGroupError); !ok {
			t.Fatal("cannot qualify absent fixture group", name, err)
		}
	}
	var groups, users []string
	t.Cleanup(func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		for i := len(users) - 1; i >= 0; i-- {
			if output, err := nativeFixtureCommand(cleanup, "/usr/sbin/userdel", users[i]); err != nil {
				t.Errorf("fixture user cleanup %s: %v %s", users[i], err, output)
			}
		}
		for i := len(groups) - 1; i >= 0; i-- {
			if _, err := user.LookupGroup(groups[i]); err != nil {
				if _, absent := err.(user.UnknownGroupError); absent {
					continue // userdel may remove its private group.
				}
				t.Errorf("fixture group lookup %s: %v", groups[i], err)
				continue
			}
			if output, err := nativeFixtureCommand(cleanup, "/usr/sbin/groupdel", groups[i]); err != nil {
				t.Errorf("fixture group cleanup %s: %v %s", groups[i], err, output)
			}
		}
	})
	for _, name := range []string{"homenode", "homenode-transfer", "homenode-runtime"} {
		if output, err := nativeFixtureCommand(ctx, "/usr/sbin/groupadd", "--system", name); err != nil {
			t.Fatal("fixture group creation", name, err, output)
		}
		groups = append(groups, name)
	}
	for _, name := range []string{"homenode", "homenode-transfer"} {
		if output, err := nativeFixtureCommand(ctx, "/usr/sbin/useradd", "--system", "--no-create-home", "--gid", name, "--groups", "homenode-runtime", "--shell", "/usr/sbin/nologin", name); err != nil {
			t.Fatal("fixture user creation", name, err, output)
		}
		users = append(users, name)
	}
	parse := func(value string) uint32 {
		n, err := strconv.ParseUint(value, 10, 32)
		if err != nil || n == 0 || n >= 65536 {
			t.Fatal("fixture service identity outside system range", value, err)
		}
		return uint32(n)
	}
	controller, err := user.Lookup("homenode")
	if err != nil {
		t.Fatal(err)
	}
	transfer, err := user.Lookup("homenode-transfer")
	if err != nil {
		t.Fatal(err)
	}
	runtimeGroup, err := user.LookupGroup("homenode-runtime")
	if err != nil {
		t.Fatal(err)
	}
	controllerUID, transferUID, runtimeGID := parse(controller.Uid), parse(transfer.Uid), parse(runtimeGroup.Gid)
	if controllerUID == transferUID || parse(controller.Gid) == runtimeGID || parse(transfer.Gid) == runtimeGID || controller.Gid == transfer.Gid {
		t.Fatal("aliased fixture service identities")
	}
	t.Logf("native Files named roles controllerUID=%d transferUID=%d runtimeGID=%d", controllerUID, transferUID, runtimeGID)
	return controllerUID, transferUID, runtimeGID
}
