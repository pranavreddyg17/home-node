package install

import "os"

// ownedAccountStepMatches accepts proxy membership only when a complete,
// canonical gateway journal owns it. Partial gateway provisioning cannot make
// configuration, maintenance or guest-account operations operational.
func (e *Engine) ownedAccountStepMatches(s accountSnapshot, base accountJournal, step int) (bool, error) {
	gateway, err := e.loadGatewayAccountJournal(base)
	if os.IsNotExist(err) {
		return accountStepMatches(s, base, step)
	}
	if err != nil || !gateway.Ready {
		return false, ErrConflict
	}
	matched, err := gatewayAccountStepMatches(s, gateway.Plan, 3)
	if err != nil || !matched {
		return false, ErrConflict
	}
	return gatewayBaseStepMatches(s, base, step)
}
