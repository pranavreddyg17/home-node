package install

import (
	"context"
	"fmt"
	"strings"
	"testing"
)

func TestResolvedGatewayMembershipRefusesNSSDrift(t *testing.T) {
	accounts := Accounts{ControllerUID: 800, ControllerGID: 800, TransferUID: 801, TransferGID: 801, RuntimeGID: 802, QEMUGID: 64055}
	gateway := GatewayAccount{UID: 803, GID: 803, ProxyGID: 804}
	passwd := map[string]string{"homenode": "800:800", "homenode-transfer": "801:801", "homenode-gateway": "803:803"}
	gids := map[string]int{"homenode": 800, "homenode-transfer": 801, "homenode-runtime": 802, "libvirt-qemu": 64055, "homenode-gateway": 803, "homenode-proxy": 804}
	for _, scenario := range []string{"valid", "gateway runtime group", "controller missing proxy", "gateway UID drift", "proxy GID drift", "resolver error"} {
		t.Run(scenario, func(t *testing.T) {
			groups := map[string]string{"homenode": "800 802 804", "homenode-transfer": "801 802", "homenode-gateway": "803 804"}
			if scenario == "gateway runtime group" {
				groups["homenode-gateway"] += " 802"
			}
			if scenario == "controller missing proxy" {
				groups["homenode"] = "800 802"
			}
			command := func(_ context.Context, program string, args ...string) ([]byte, error) {
				if scenario == "resolver error" {
					return nil, ErrAccounts
				}
				if program == "/usr/bin/id" {
					return []byte(groups[args[1]] + "\n"), nil
				}
				if program != "/usr/bin/getent" || len(args) != 2 {
					t.Fatal("unexpected resolver command")
				}
				name := args[1]
				if args[0] == "passwd" {
					pair := passwd[name]
					if scenario == "gateway UID drift" && name == "homenode-gateway" {
						pair = "805:803"
					}
					return []byte(name + ":x:" + pair + ":fixture:/nonexistent:/usr/sbin/nologin\n"), nil
				}
				gid := gids[name]
				if scenario == "proxy GID drift" && name == "homenode-proxy" {
					gid = 805
				}
				return []byte(fmt.Sprintf("%s:x:%d:\n", name, gid)), nil
			}
			err := resolvedAccountsWith(context.Background(), accounts, &gateway, command)
			if (err == nil) != (scenario == "valid") {
				t.Fatal("NSS drift result", scenario, err)
			}
		})
	}
	// Legacy installations retain exactly their original supplementary groups.
	command := func(_ context.Context, program string, args ...string) ([]byte, error) {
		name := args[1]
		if strings.Contains(name, "gateway") || strings.Contains(name, "proxy") {
			t.Fatal("legacy lookup requires extension")
		}
		if program == "/usr/bin/id" {
			return []byte(fmt.Sprintf("%d 802", gids[name])), nil
		}
		if args[0] == "passwd" {
			return []byte(name + ":x:" + passwd[name] + ":fixture:/nonexistent:/usr/sbin/nologin\n"), nil
		}
		return []byte(fmt.Sprintf("%s:x:%d:\n", name, gids[name])), nil
	}
	if err := resolvedAccountsWith(context.Background(), accounts, nil, command); err != nil {
		t.Fatal(err)
	}
}
