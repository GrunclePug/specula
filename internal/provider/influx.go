package provider

import (
	"bytes"
	"context"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/grunclepug/specula/internal/db"
)

// InfluxProvider ships metrics to an InfluxDB instance using the line protocol
// and caches current status locally in-memory synced with the SQLite host list.
type InfluxProvider struct {
	database   *db.DB
	url        string
	token      string
	org        string
	bucket     string
	httpClient *http.Client

	mu     sync.RWMutex
	status map[string]HostStatus
}

// NewInfluxProvider creates a new InfluxProvider.
func NewInfluxProvider(database *db.DB, urlStr, token, org, bucket string) (*InfluxProvider, error) {
	u, err := url.Parse(urlStr)
	if err != nil {
		return nil, fmt.Errorf("invalid InfluxDB URL: %w", err)
	}

	// Verify required params
	if token == "" {
		return nil, fmt.Errorf("InfluxDB token is required")
	}
	if org == "" {
		return nil, fmt.Errorf("InfluxDB organization is required")
	}
	if bucket == "" {
		return nil, fmt.Errorf("InfluxDB bucket is required")
	}

	return &InfluxProvider{
		database: database,
		url:      u.String(),
		token:    token,
		org:      org,
		bucket:   bucket,
		httpClient: &http.Client{
			Timeout: 5 * time.Second,
		},
		status: make(map[string]HostStatus),
	}, nil
}

// SaveStatus updates the in-memory cache and ships the status metric to InfluxDB.
func (p *InfluxProvider) SaveStatus(host, name string, groups []string, loss float64, rtt time.Duration) error {
	now := time.Now()

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

	// Update local cache
	p.mu.Lock()
	p.status[host] = HostStatus{
		Host:      host,
		Name:      name,
		Groups:    groups,
		Up:        up,
		Loss:      loss,
		Status:    status,
		RTT:       rtt,
		LastCheck: now,
	}
	p.mu.Unlock()

	// Ship to InfluxDB in a separate goroutine to prevent blocking the monitor ticker
	go func() {
		if err := p.shipMetric(host, name, groups, up, loss, status, rtt, now); err != nil {
			log.Printf("[InfluxDB] error shipping metric for host %s: %v", host, err)
		}
	}()

	return nil
}

// GetStatus returns the current status of all hosts defined in SQLite.
func (p *InfluxProvider) GetStatus() ([]HostStatus, error) {
	return p.GetStatusFiltered("")
}

// GetStatusFiltered returns the status of hosts filtered by a group name.
func (p *InfluxProvider) GetStatusFiltered(groupFilter string) ([]HostStatus, error) {
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

func (p *InfluxProvider) shipMetric(host, name string, groups []string, up bool, loss float64, status string, rtt time.Duration, t time.Time) error {
	upVal := "false"
	if up {
		upVal = "true"
	}
	rttMs := float64(rtt) / float64(time.Millisecond)
	grpsVal := strings.Join(groups, ",")

	// Format InfluxDB Line Protocol (escape string fields with quotes)
	line := fmt.Sprintf("ping,host=%s up=%s,name=\"%s\",groups=\"%s\",loss=%.2f,status=\"%s\",rtt_ms=%.3f %d\n", host, upVal, name, grpsVal, loss, status, rttMs, t.UnixNano())

	// Build the API URL for write endpoint
	writeURL, err := url.Parse(p.url)
	if err != nil {
		return err
	}

	// Ensure path points to /api/v2/write
	if writeURL.Path == "" || writeURL.Path == "/" {
		writeURL.Path = "/api/v2/write"
	}

	q := writeURL.Query()
	q.Set("org", p.org)
	q.Set("bucket", p.bucket)
	q.Set("precision", "ns")
	writeURL.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(context.Background(), "POST", writeURL.String(), bytes.NewBufferString(line))
	if err != nil {
		return err
	}

	req.Header.Set("Authorization", "Token "+p.token)
	req.Header.Set("Content-Type", "text/plain; charset=utf-8")

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		buf := new(bytes.Buffer)
		_, _ = buf.ReadFrom(resp.Body)
		return fmt.Errorf("received status code %d: %s", resp.StatusCode, buf.String())
	}

	return nil
}
