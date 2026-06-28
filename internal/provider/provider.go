package provider

import "time"

// HostStatus represents the current status of a pinged target, including latency, loss, and groups.
type HostStatus struct {
	Host      string        `json:"host"`
	Name      string        `json:"name"`   // friendly hostname/label
	Groups    []string      `json:"groups"` // associated groups list
	Up        bool          `json:"up"`     // true if loss < 100%
	Loss      float64       `json:"loss"`
	Status    string        `json:"status"` // "healthy", "degraded", "offline"
	RTT       time.Duration `json:"rtt"`
	LastCheck time.Time     `json:"last_check"`
}

// MetricProvider defines the interface for saving ping metrics and retrieving
// the current status of all targets.
type MetricProvider interface {
	SaveStatus(host, name string, groups []string, loss float64, rtt time.Duration) error
	GetStatus() ([]HostStatus, error)
	GetStatusFiltered(group string) ([]HostStatus, error)
}



