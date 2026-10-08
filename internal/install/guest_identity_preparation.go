package install

// Preparation identifies exact approved bytes; it is not host configuration,
// allocation exclusivity, runtime policy publication or activation authority.
type GuestIdentityConfigurationPreview struct {
	OwnerID              string `json:"ownerId"`
	OriginalSHA256       string `json:"originalSha256"`
	DesiredSHA256        string `json:"desiredSha256"`
	ChangesRequired      bool   `json:"changesRequired"`
	IntentCommitted      bool   `json:"intentCommitted"`
	ConfigurationApplied bool   `json:"configurationApplied"`
}
