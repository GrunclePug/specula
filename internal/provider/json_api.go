package provider

import (
	"sync"
	"time"

	"github.com/grunclepug/specula/internal/db"
)

// JSONAPIProvider stores metrics in memory and synchronizes its active host list with SQLite.
type JSONAPIProvider struct {
	database *db.DB
	mu       sync.RWMutex
	status   map[string]HostStatus
}

// NewJSONAPIProvider initializes a new JSONAPIProvider.
func NewJSONAPIProvider(database *db.DB) *JSONAPIProvider {
	return &JSONAPIProvider{
		database: database,
		status:   make(map[string]HostStatus),
	}
}

// SaveStatus records the metrics and determines status based on packet loss.
func (p *JSONAPIProvider) SaveStatus(host, name string, groups []string, loss float64, rtt time.Duration) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	up := true
	var status string
	if loss >= 100 {
		up = false
		status = "offline"
	} else if loss > 0 {
		status = "degraded"
	} else {
		status = "healthy"
	}

	p.status[host] = HostStatus{
		Host:      host,
		Name:      name,
		Groups:    groups,
		Up:        up,
		Loss:      loss,
		Status:    status,
		RTT:       rtt,
		LastCheck: time.Now(),
	}
	return nil
}

// GetStatus returns the current status of all hosts defined in SQLite.
func (p *JSONAPIProvider) GetStatus() ([]HostStatus, error) {
	return p.GetStatusFiltered("")
}

// GetStatusFiltered returns the status of hosts filtered by a group name.
func (p *JSONAPIProvider) GetStatusFiltered(groupFilter string) ([]HostStatus, error) {
	hosts, err := p.database.GetHostsFiltered(groupFilter)
	if err != nil {
		return nil, err
	}

	p.mu.RLock()
	defer p.mu.RUnlock()

	list := make([]HostStatus, 0, len(hosts))
	for _, h := range hosts {
		if s, ok := p.status[h.Host]; ok {
			list = append(list, s)
		} else {
			// Target exists in SQLite but has not been pinged by the monitor yet
			list = append(list, HostStatus{
				Host:   h.Host,
				Name:   h.Name,
				Groups: h.Groups,
				Up:     false,
				Loss:   0,
				Status: "pending",
			})
		}
	}

	return list, nil
}
