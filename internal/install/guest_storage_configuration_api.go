package install

// GuestStorageConfigurationMigrationResult reports configuration publication.
// Service activation remains a separately qualified operation.
type GuestStorageConfigurationMigrationResult struct {
	PolicyPublished               bool `json:"policyPublished"`
	EnvironmentPublished          bool `json:"environmentPublished"`
	ConfigurationJournalPublished bool `json:"configurationJournalPublished"`
	ServicesActivated             bool `json:"servicesActivated"`
	ActivationQualified           bool `json:"activationQualified"`
}
