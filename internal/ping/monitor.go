package ping

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/grunclepug/specula/internal/db"
	"github.com/grunclepug/specula/internal/provider"
)

// Monitor orchestrates the periodic ping checks for all targets.
type Monitor struct {
	database  *db.DB
	prov      provider.MetricProvider
	interval  time.Duration
	timeout   time.Duration
	pingCount int
}

// NewMonitor creates a new status monitor.
func NewMonitor(database *db.DB, prov provider.MetricProvider, interval, timeout time.Duration, pingCount int) *Monitor {
	return &Monitor{
		database:  database,
		prov:      prov,
		interval:  interval,
		timeout:   timeout,
		pingCount: pingCount,
	}
}

// Start runs the monitor loop in a blocking call, listening to context cancellation.
func (m *Monitor) Start(ctx context.Context) {
	slog.Info("[Monitor] Starting ping status monitor", "interval", m.interval, "timeout", m.timeout, "packets", m.pingCount)

	// Run initial check immediately
	m.checkAll()

	ticker := time.NewTicker(m.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			slog.Info("[Monitor] Shutting down status monitor...")
			return
		case <-ticker.C:
			m.checkAll()
		}
	}
}

// checkAll queries the database for hosts and checks them concurrently.
func (m *Monitor) checkAll() {
	targets, err := m.database.GetHosts()
	if err != nil {
		slog.Error("[Monitor] Error loading targets from database", "error", err)
		return
	}

	var wg sync.WaitGroup
	for _, target := range targets {
		wg.Add(1)
		go func(t db.TargetHost) {
			defer wg.Done()
			loss, rtt, err := Ping(t.Host, m.timeout, m.pingCount)
			if err != nil {
				slog.Warn("[Monitor] Ping check failed", "host", t.Host, "error", err)
				// If checking failed, make sure we default to 100% loss
				loss = 100
				rtt = 0
			}
			if err := m.prov.SaveStatus(t.Host, t.Name, t.Groups, loss, rtt); err != nil {
				slog.Error("[Monitor] Failed to save status for target", "host", t.Host, "error", err)
			}
		}(target)
	}
	wg.Wait()
}


