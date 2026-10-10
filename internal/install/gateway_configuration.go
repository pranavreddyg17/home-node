package install

import (
	"bytes"
	"context"
	"os"
)

// gatewayControlUnit removes controller TCP/TLS authority from the reviewed
// source unit. Applying it requires a verified, owned gateway identity.
func gatewayControlUnit(data []byte) ([]byte, error) {
	replacements := [][2]string{
		{"SupplementaryGroups=homenode-runtime\n", "SupplementaryGroups=homenode-runtime homenode-proxy\n"},
		{" --tls-cert /etc/homenode/tls/server.crt --tls-key /etc/homenode/tls/server.key ", " --controller-socket /run/homenode-control/control.sock --gateway-uid ${GATEWAY_UID} --access-gid ${PROXY_GID} "},
		{"StateDirectory=homenode/control\n", "RuntimeDirectory=homenode-control\nRuntimeDirectoryMode=0755\nStateDirectory=homenode/control\n"},
		{"RestrictAddressFamilies=AF_UNIX AF_INET AF_INET6 AF_NETLINK\n", "RestrictAddressFamilies=AF_UNIX\n"},
		{"ReadWritePaths=/var/lib/homenode/control\n", "ReadWritePaths=/var/lib/homenode/control /run/homenode-control\n"},
		{"InaccessiblePaths=", "InaccessiblePaths=-/etc/homenode/tls "},
	}
	for _, replacement := range replacements {
		before, after := []byte(replacement[0]), []byte(replacement[1])
		if bytes.Count(data, before) != 1 {
			return nil, ErrPlan
		}
		data = bytes.Replace(data, before, after, 1)
	}
	return data, nil
}

func (e *Engine) observeGatewayAccount(ctx context.Context, base accountJournal) (*GatewayAccount, error) {
	return e.observeGatewayAccountWith(ctx, base, nativeGatewayProvisioner{})
}

func (e *Engine) observeGatewayAccountWith(ctx context.Context, base accountJournal, b gatewayProvisionBackend) (*GatewayAccount, error) {
	j, err := e.loadGatewayAccountJournal(base)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if !j.Ready {
		return nil, ErrAccounts
	}
	snapshot, err := b.Snapshot(ctx)
	defer clear(snapshot.shadow)
	if err != nil {
		return nil, err
	}
	for step := 0; step < 4; step++ {
		matched, err := gatewayAccountStepMatches(snapshot, j.Plan, step)
		if err != nil || !matched {
			return nil, ErrAccounts
		}
	}
	for step := 0; step < 5; step++ {
		matched, err := gatewayBaseStepMatches(snapshot, base, step)
		if err != nil || !matched {
			return nil, ErrAccounts
		}
	}
	identity, err := b.VerifyGateway(ctx)
	if err != nil {
		return nil, err
	}
	if identity != j.Plan.Identity {
		return nil, ErrAccounts
	}
	return &identity, nil
}
