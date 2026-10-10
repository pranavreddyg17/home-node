package install

// EmptyVolumeStorageMigrationResult describes the completed empty volume-parent phase.
// It is not a runtime-policy or service-activation qualification.
type EmptyVolumeStorageMigrationResult struct {
	EmptyParentOwnershipMigrated bool `json:"emptyParentOwnershipMigrated"`
	ParentJournalPublished       bool `json:"parentJournalPublished"`
	PopulatedVolumesMigrated     bool `json:"populatedVolumesMigrated"`
	PolicyPublished              bool `json:"policyPublished"`
	ServicesActivated            bool `json:"servicesActivated"`
	ActivationQualified          bool `json:"activationQualified"`
}
