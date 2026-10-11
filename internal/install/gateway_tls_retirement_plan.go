package install

import "encoding/hex"

// TLSOwnershipChange changes permissions on a retained inode; it never copies,
// deletes or replaces TLS contents. Native admission must pin each source inode.
type TLSOwnershipChange struct {
	Path       string `json:"path"`
	BeforeMode uint32 `json:"beforeMode"`
	BeforeGID  int    `json:"beforeGid"`
	AfterMode  uint32 `json:"afterMode"`
	AfterGID   int    `json:"afterGid"`
}
type GatewayTLSRetirementPlan struct {
	InstallationID     string                   `json:"installationId"`
	InstallationDigest string                   `json:"installationDigest"`
	Identity           GatewayAccount           `json:"identity"`
	Key                ConfigurationRemovalFile `json:"key"`
	Certificate        ConfigurationRemovalFile `json:"certificate"`
	Changes            []TLSOwnershipChange     `json:"changes"`
}

func gatewayTLSRetirementPlan(installed journal, identity GatewayAccount, key, certificate ConfigurationRemovalFile) (GatewayTLSRetirementPlan, error) {
	var empty GatewayTLSRetirementPlan
	if installed.Phase != "installed" || identity.UID < 100 || identity.UID > 999 || identity.GID < 100 || identity.GID > 999 || identity.ProxyGID < 100 || identity.ProxyGID > 999 || identity.GID == identity.ProxyGID {
		return empty, ErrConflict
	}
	if key.Path != "etc/homenode/tls/server.key" || certificate.Path != "etc/homenode/tls/server.crt" || key.UID != 0 || certificate.UID != 0 || key.Mode != 0640 || key.GID != identity.GID || certificate.Mode != 0644 || certificate.GID < 0 || certificate.GID > 1<<31-1 {
		return empty, ErrConflict
	}
	for _, file := range []ConfigurationRemovalFile{key, certificate} {
		if len(file.SHA256) != 64 {
			return empty, ErrConflict
		}
		if _, err := hex.DecodeString(file.SHA256); err != nil {
			return empty, ErrConflict
		}
	}
	references := gatewayRemovalIdentityReferences(installed, identity)
	if len(references) != 1 || references[0] != "etc/homenode/tls" {
		return empty, ErrConflict
	}
	admitted := false
	for _, item := range installed.Items {
		if item.Path == "etc/homenode/tls" {
			if admitted || !item.Directory || item.UID != 0 || item.GID != identity.GID || item.Mode != 0750 || item.State != "created" {
				return empty, ErrConflict
			}
			admitted = true
		}
	}
	if !admitted {
		return empty, ErrConflict
	}
	return GatewayTLSRetirementPlan{InstallationID: installed.ID, InstallationDigest: installed.Digest, Identity: identity, Key: key, Certificate: certificate, Changes: []TLSOwnershipChange{
		{key.Path, 0640, identity.GID, 0600, identity.GID},
		{key.Path, 0600, identity.GID, 0600, 0},
		{certificate.Path, 0644, certificate.GID, 0644, 0},
		{"etc/homenode/tls", 0750, identity.GID, 0700, identity.GID},
		{"etc/homenode/tls", 0700, identity.GID, 0700, 0},
	}}, nil
}
