package supervisor

// ReservedGuestPolicy is independently published host configuration. It is not
// inferred from leases or ownership receipts and contains no mutable conflicts.
// Validation alone does not attest account reservation, KVM group or storage.
type ReservedGuestPolicy struct {
	Version  int    `json:"version"`
	FirstUID uint32 `json:"firstUid"`
	LastUID  uint32 `json:"lastUid"`
	GuestGID uint32 `json:"guestGid"`
}

func (p ReservedGuestPolicy) Validate(serviceUIDs ...uint32) error {
	if p.Version != 1 || p.GuestGID == 0 || p.GuestGID > 1<<31-1 {
		return ErrPolicy
	}
	if err := (GuestUIDPool{First: p.FirstUID, Last: p.LastUID}).validate(); err != nil {
		return err
	}
	for _, uid := range serviceUIDs {
		if uid == 0 || uid >= p.FirstUID && uid <= p.LastUID {
			return ErrPolicy
		}
	}
	return nil
}
