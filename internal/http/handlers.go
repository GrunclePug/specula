package http

import (
	"bytes"
	"encoding/json"
	"fmt"
	"html/template"
	"log/slog"
	"net"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/grunclepug/specula/internal/db"
	"github.com/grunclepug/specula/internal/provider"
	"github.com/grunclepug/specula/internal/web"
)

// Handler handles all dashboard and API HTTP requests.
type Handler struct {
	database  *db.DB
	prov      provider.MetricProvider
	version   string
	templates map[string]*template.Template
}

// StatsSummary groups basic stats to update summary cards.
type StatsSummary struct {
	Total int
	Up    int
	Down  int
}

// PageData wraps variables passed to the index dashboard template.
type PageData struct {
	Hosts        []provider.HostStatus
	Stats        StatsSummary
	Version      string
	Groups       []string
	CurrentGroup string
	CurrentSort  string
}

// TargetsPageData wraps variables passed to the targets management page template.
type TargetsPageData struct {
	Hosts       []db.TargetHost
	Version     string
	CurrentSort string
}

// GroupsPageData wraps variables passed to the groups management page template.
type GroupsPageData struct {
	Groups  []db.GroupInfo
	Version string
}

// NewHandler initializes templates and creates a new Handler instance.
func NewHandler(database *db.DB, prov provider.MetricProvider, version string) (*Handler, error) {
	funcMap := template.FuncMap{
		"formatRTT": func(d time.Duration) string {
			if d == 0 {
				return "—"
			}
			if d < time.Millisecond {
				return fmt.Sprintf("%.2f µs", float64(d.Microseconds()))
			}
			return fmt.Sprintf("%.2f ms", float64(d.Nanoseconds())/1e6)
		},
		"formatTime": func(t time.Time) string {
			if t.IsZero() {
				return "Never"
			}
			return t.Format("15:04:05")
		},
		"formatLoss": func(l float64) string {
			return fmt.Sprintf("%.0f%%", l)
		},
		"join": func(s []string, sep string) string {
			return strings.Join(s, sep)
		},
	}

	templates := make(map[string]*template.Template)

	// Helper to parse base layout + components + page templates isolatedly
	parseTmpl := func(name string, pageFile string) error {
		t, err := template.New(name).Funcs(funcMap).ParseFS(
			web.Files,
			"templates/layouts/base.html",
			"templates/components/status_row.html",
			pageFile,
		)
		if err != nil {
			return err
		}
		templates[name] = t
		return nil
	}

	if err := parseTmpl("index", "templates/index.html"); err != nil {
		return nil, fmt.Errorf("failed to parse index template: %w", err)
	}
	if err := parseTmpl("targets", "templates/targets.html"); err != nil {
		return nil, fmt.Errorf("failed to parse targets template: %w", err)
	}
	if err := parseTmpl("groups", "templates/groups.html"); err != nil {
		return nil, fmt.Errorf("failed to parse groups template: %w", err)
	}

	return &Handler{
		database:  database,
		prov:      prov,
		version:   version,
		templates: templates,
	}, nil
}

// GetDashboard serves the main dashboard HTML page (visual status only).
func (h *Handler) GetDashboard(w http.ResponseWriter, r *http.Request) {
	groupFilter := r.URL.Query().Get("group")
	sortVal := r.URL.Query().Get("sort")
	if sortVal == "" {
		sortVal = "ip"
	}

	hosts, err := h.prov.GetStatusFiltered(groupFilter)
	if err != nil {
		slog.Error("[HTTP] Error getting status", "error", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	if sortVal == "name" {
		slices.SortFunc(hosts, func(a, b provider.HostStatus) int {
			return compareNames(a.Name, b.Name, a.Host, b.Host)
		})
	} else {
		slices.SortFunc(hosts, func(a, b provider.HostStatus) int {
			return compareIPs(a.Host, b.Host)
		})
	}

	groups, err := h.database.GetGroups()
	if err != nil {
		slog.Error("[HTTP] Error getting groups list", "error", err)
	}

	data := PageData{
		Hosts:        hosts,
		Stats:        computeStats(hosts),
		Version:      h.version,
		Groups:       groups,
		CurrentGroup: groupFilter,
		CurrentSort:  sortVal,
	}

	var buf bytes.Buffer
	if err := h.templates["index"].ExecuteTemplate(&buf, "index.html", data); err != nil {
		slog.Error("[HTTP] Error executing dashboard template", "error", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = buf.WriteTo(w)
}

// GetStatusTable serves the HTMX-polled status rows and stats out-of-band updates.
func (h *Handler) GetStatusTable(w http.ResponseWriter, r *http.Request) {
	groupFilter := r.URL.Query().Get("group")
	sortVal := r.URL.Query().Get("sort")
	if sortVal == "" {
		sortVal = "ip"
	}

	hosts, err := h.prov.GetStatusFiltered(groupFilter)
	if err != nil {
		slog.Error("[HTTP] Error getting status table data", "error", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	if sortVal == "name" {
		slices.SortFunc(hosts, func(a, b provider.HostStatus) int {
			return compareNames(a.Name, b.Name, a.Host, b.Host)
		})
	} else {
		slices.SortFunc(hosts, func(a, b provider.HostStatus) int {
			return compareIPs(a.Host, b.Host)
		})
	}

	data := PageData{
		Hosts:        hosts,
		Stats:        computeStats(hosts),
		Version:      h.version,
		CurrentGroup: groupFilter,
		CurrentSort:  sortVal,
	}

	var buf bytes.Buffer
	if err := h.templates["index"].ExecuteTemplate(&buf, "status_rows", data); err != nil {
		slog.Error("[HTTP] Error executing status_rows template", "error", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = buf.WriteTo(w)
}

// GetTargetsView serves the target hosts management HTML page.
func (h *Handler) GetTargetsView(w http.ResponseWriter, r *http.Request) {
	sortVal := r.URL.Query().Get("sort")
	if sortVal == "" {
		sortVal = "ip"
	}

	hosts, err := h.database.GetHosts()
	if err != nil {
		slog.Error("[HTTP] Error getting hosts for targets page", "error", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	if sortVal == "name" {
		slices.SortFunc(hosts, func(a, b db.TargetHost) int {
			return compareNames(a.Name, b.Name, a.Host, b.Host)
		})
	} else {
		slices.SortFunc(hosts, func(a, b db.TargetHost) int {
			return compareIPs(a.Host, b.Host)
		})
	}

	data := TargetsPageData{
		Hosts:       hosts,
		Version:     h.version,
		CurrentSort: sortVal,
	}

	var buf bytes.Buffer
	if err := h.templates["targets"].ExecuteTemplate(&buf, "targets.html", data); err != nil {
		slog.Error("[HTTP] Error executing targets view template", "error", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = buf.WriteTo(w)
}

// GetGroupsView serves the groups management HTML page.
func (h *Handler) GetGroupsView(w http.ResponseWriter, r *http.Request) {
	groups, err := h.database.GetGroupsInfo()
	if err != nil {
		slog.Error("[HTTP] Error getting groups info for groups page", "error", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	data := GroupsPageData{
		Groups:  groups,
		Version: h.version,
	}

	var buf bytes.Buffer
	if err := h.templates["groups"].ExecuteTemplate(&buf, "groups.html", data); err != nil {
		slog.Error("[HTTP] Error executing groups view template", "error", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = buf.WriteTo(w)
}

// AddHost inserts a host and friendly name/groups and redirects back to referrer page.
func (h *Handler) AddHost(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		slog.Error("[HTTP] AddHost form parsing error", "error", err)
		http.Error(w, "Bad Request", http.StatusBadRequest)
		return
	}

	host := strings.TrimSpace(r.FormValue("host"))
	name := strings.TrimSpace(r.FormValue("name"))
	groupsStr := r.FormValue("groups")
	oldHost := strings.TrimSpace(r.FormValue("old_host"))

	if host == "" {
		http.Error(w, "Host target is required", http.StatusBadRequest)
		return
	}

	var groups []string
	for _, g := range strings.Split(groupsStr, ",") {
		g = strings.TrimSpace(g)
		if g != "" {
			groups = append(groups, g)
		}
	}

	var err error
	if oldHost != "" {
		err = h.database.UpdateHost(oldHost, host, name, groups)
	} else {
		err = h.database.AddHost(host, name, groups)
	}

	if err != nil {
		slog.Error("[HTTP] Failed to save host to database", "host", host, "old_host", oldHost, "error", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	// Support POST-Redirect-Get pattern (especially for HTMX updates)
	if r.Header.Get("HX-Request") == "true" {
		w.Header().Set("HX-Redirect", "/targets")
		w.WriteHeader(http.StatusOK)
		return
	}

	http.Redirect(w, r, "/targets", http.StatusSeeOther)
}

// DeleteHost removes a host from SQLite DB and redirects back to referrer page.
func (h *Handler) DeleteHost(w http.ResponseWriter, r *http.Request) {
	host := r.URL.Query().Get("host")
	if host == "" {
		http.Error(w, "Host parameter is required", http.StatusBadRequest)
		return
	}

	if err := h.database.DeleteHost(host); err != nil {
		slog.Error("[HTTP] Failed to delete host", "host", host, "error", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	if r.Header.Get("HX-Request") == "true" {
		w.Header().Set("HX-Redirect", "/targets")
		w.WriteHeader(http.StatusOK)
		return
	}

	http.Redirect(w, r, "/targets", http.StatusSeeOther)
}

// AddGroup creates a new group and redirects back to the groups page.
func (h *Handler) AddGroup(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		slog.Error("[HTTP] AddGroup form parsing error", "error", err)
		http.Error(w, "Bad Request", http.StatusBadRequest)
		return
	}

	groupName := strings.TrimSpace(r.FormValue("group"))
	if groupName == "" {
		http.Error(w, "Group name is required", http.StatusBadRequest)
		return
	}

	if err := h.database.CreateGroup(groupName); err != nil {
		slog.Error("[HTTP] Failed to create group", "group", groupName, "error", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	if r.Header.Get("HX-Request") == "true" {
		w.Header().Set("HX-Redirect", "/groups")
		w.WriteHeader(http.StatusOK)
		return
	}

	http.Redirect(w, r, "/groups", http.StatusSeeOther)
}

// DeleteGroup removes a group and redirects back to the groups page.
func (h *Handler) DeleteGroup(w http.ResponseWriter, r *http.Request) {
	group := r.URL.Query().Get("group")
	if group == "" {
		http.Error(w, "Group parameter is required", http.StatusBadRequest)
		return
	}

	if err := h.database.DeleteGroup(group); err != nil {
		slog.Error("[HTTP] Failed to delete group", "group", group, "error", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	if r.Header.Get("HX-Request") == "true" {
		w.Header().Set("HX-Redirect", "/groups")
		w.WriteHeader(http.StatusOK)
		return
	}

	http.Redirect(w, r, "/groups", http.StatusSeeOther)
}

// GetHealthz responds with status 200 OK (liveness probe).
func (h *Handler) GetHealthz(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"status":    "alive",
		"version":   h.version,
		"timestamp": time.Now().Format(time.RFC3339),
	})
}

// GetReadyz checks database connection (readiness probe).
func (h *Handler) GetReadyz(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if err := h.database.Ping(); err != nil {
		slog.Error("[HTTP] Readiness check failed", "error", err)
		w.WriteHeader(http.StatusServiceUnavailable)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"status":    "unavailable",
			"error":     err.Error(),
			"timestamp": time.Now().Format(time.RFC3339),
		})
		return
	}

	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"status":    "ready",
		"timestamp": time.Now().Format(time.RFC3339),
	})
}

// GetAPIStatus returns the status of targets in JSON format (supports ?group=..., ?host=..., ?ip=..., ?name=... query filtering).
func (h *Handler) GetAPIStatus(w http.ResponseWriter, r *http.Request) {
	groupFilter := r.URL.Query().Get("group")
	hostFilter := r.URL.Query().Get("host")
	ipFilter := r.URL.Query().Get("ip")
	nameFilter := r.URL.Query().Get("name")

	// Use 'ip' parameter as an alias for 'host' if 'host' is empty
	if hostFilter == "" {
		hostFilter = ipFilter
	}

	hosts, err := h.prov.GetStatusFiltered(groupFilter)
	if err != nil {
		slog.Error("[HTTP] Error getting status for API", "error", err)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "failed to retrieve status"})
		return
	}

	// Apply case-insensitive partial filters for host/ip and name
	if hostFilter != "" || nameFilter != "" {
		var filtered []provider.HostStatus
		hostFilterLower := strings.ToLower(hostFilter)
		nameFilterLower := strings.ToLower(nameFilter)

		for _, hs := range hosts {
			match := true
			if hostFilter != "" && !strings.Contains(strings.ToLower(hs.Host), hostFilterLower) {
				match = false
			}
			if nameFilter != "" && !strings.Contains(strings.ToLower(hs.Name), nameFilterLower) {
				match = false
			}
			if match {
				filtered = append(filtered, hs)
			}
		}
		hosts = filtered
	}

	// Sort naturally by IP address ascending by default for API consistency
	slices.SortFunc(hosts, func(a, b provider.HostStatus) int {
		return compareIPs(a.Host, b.Host)
	})

	if hosts == nil {
		hosts = []provider.HostStatus{}
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(hosts); err != nil {
		slog.Error("[HTTP] Error encoding API status response", "error", err)
	}
}

func computeStats(hosts []provider.HostStatus) StatsSummary {
	var total, up, down int
	for _, h := range hosts {
		total++
		if h.Status != "offline" {
			up++
		} else {
			down++
		}
	}
	return StatsSummary{
		Total: total,
		Up:    up,
		Down:  down,
	}
}

// compareIPs parses and compares two host strings (IP address or hostname) naturally.
func compareIPs(h1, h2 string) int {
	cleanHost := func(host string) string {
		if sh, _, err := net.SplitHostPort(host); err == nil {
			return sh
		}
		return host
	}

	c1 := cleanHost(h1)
	c2 := cleanHost(h2)

	ip1 := net.ParseIP(c1)
	ip2 := net.ParseIP(c2)

	// If both are valid IP addresses
	if ip1 != nil && ip2 != nil {
		ip1_4 := ip1.To4()
		ip2_4 := ip2.To4()

		if ip1_4 != nil && ip2_4 != nil {
			return bytes.Compare(ip1_4, ip2_4)
		}
		if ip1_4 != nil && ip2_4 == nil {
			return -1 // IPv4 first
		}
		if ip1_4 == nil && ip2_4 != nil {
			return 1 // IPv6 last
		}
		// Both are IPv6
		return bytes.Compare(ip1, ip2)
	}

	// If one is a valid IP and the other is a hostname, put IPs first
	if ip1 != nil && ip2 == nil {
		return -1
	}
	if ip1 == nil && ip2 != nil {
		return 1
	}

	// Both are hostnames, compare lexicographically
	return strings.Compare(c1, c2)
}

// compareNames compares friendly names alphabetically, falling back to IP order if identical.
func compareNames(n1, n2, h1, h2 string) int {
	comp := strings.Compare(strings.ToLower(n1), strings.ToLower(n2))
	if comp != 0 {
		return comp
	}
	return compareIPs(h1, h2)
}

