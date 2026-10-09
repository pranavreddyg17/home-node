package supervisor

// Retain admission through both destroy and cooperative poweroff calls. A
// completed parallel stop cannot authorize restart while either remains live.
func (m *Manager) retainStopEffect(id string) func() {
	m.stopMu.Lock()
	if m.activeStops == nil {
		m.activeStops = make(map[string]int)
	}
	m.activeStops[id]++
	m.stopMu.Unlock()
	return func() {
		m.stopMu.Lock()
		m.activeStops[id]--
		if m.activeStops[id] == 0 {
			delete(m.activeStops, id)
		}
		m.stopMu.Unlock()
	}
}
