package install

// ImageStorageMigrationResult describes the completed image-storage phase.
// It is not a runtime-policy or service-activation qualification.
type ImageStorageMigrationResult struct {
	ImageOwnershipMigrated bool `json:"imageOwnershipMigrated"`
	ParentJournalPublished bool `json:"parentJournalPublished"`
	PolicyPublished        bool `json:"policyPublished"`
	ServicesActivated      bool `json:"servicesActivated"`
	ActivationQualified    bool `json:"activationQualified"`
}
