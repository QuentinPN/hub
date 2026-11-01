// Package domain holds the inventory types shared by the API and, later, the discovery code.
package domain

// Status is what the dashboard shows for a service.
type Status string

const (
	StatusOnline         Status = "online"
	StatusDegraded       Status = "degraded"
	StatusDown           Status = "down"
	StatusUnknown        Status = "unknown"
	StatusConfigRequired Status = "config_required"
)

// EffectiveStatus is the status to display given what was last observed and whether the
// host's agent is currently reachable. A silent agent says nothing about the service, so
// it must never be reported as down (PRODUCT.md, acceptance criterion 5).
func EffectiveStatus(observed Status, hostReachable bool) Status {
	if !hostReachable {
		return StatusUnknown
	}
	return observed
}
