package install

// GuestStorageChannelProvisioningResult describes one runtime-directory phase.
// Reboot provisioning and activation require separate qualification.
type GuestStorageChannelProvisioningResult struct {
	ChannelParentPublished bool `json:"channelParentPublished"`
	ServicesActivated      bool `json:"servicesActivated"`
	ActivationQualified    bool `json:"activationQualified"`
}
