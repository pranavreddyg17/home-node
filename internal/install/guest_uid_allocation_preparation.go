package install

type GuestUIDAllocationPreview struct {
	OwnerID              string                  `json:"ownerId"`
	First                uint32                  `json:"firstUid"`
	Last                 uint32                  `json:"lastUid"`
	Selection            GuestUIDAllocatorRanges `json:"selection"`
	OriginalSHA256       string                  `json:"originalSha256"`
	DesiredSHA256        string                  `json:"desiredSha256"`
	ChangesRequired      bool                    `json:"changesRequired"`
	IntentCommitted      bool                    `json:"intentCommitted"`
	ConfigurationApplied bool                    `json:"configurationApplied"`
}
