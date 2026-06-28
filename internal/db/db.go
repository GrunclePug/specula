package db

import (
	"database/sql"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	_ "modernc.org/sqlite"
)

// TargetHost represents a host definition from the database with associated groups.
type TargetHost struct {
	Host   string
	Name   string
	Groups []string
}

// DB wraps the SQLite sql.DB connection.
type DB struct {
	conn *sql.DB
}

// New initializes the SQLite connection and runs migration steps.
func New(path string) (*DB, error) {
	// Ensure parent directory exists (critical for container PV volume mounts)
	dir := filepath.Dir(path)
	if dir != "." && dir != "/" {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return nil, fmt.Errorf("failed to create database directory %q: %w", dir, err)
		}
	}

	conn, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("failed to open sqlite database: %w", err)
	}

	db := &DB{conn: conn}
	if err := db.init(); err != nil {
		_ = conn.Close()
		return nil, err
	}

	return db, nil
}

// Close closes the database connection.
func (db *DB) Close() error {
	return db.conn.Close()
}

// Ping checks if the database connection is alive.
func (db *DB) Ping() error {
	return db.conn.Ping()
}

func (db *DB) init() error {
	// Enable foreign key support in SQLite
	_, err := db.conn.Exec("PRAGMA foreign_keys = ON;")
	if err != nil {
		return fmt.Errorf("failed to enable foreign keys pragma: %w", err)
	}

	// Migration check: check if the 'host_groups' table exists
	var tableExists int
	_ = db.conn.QueryRow("SELECT count(*) FROM sqlite_master WHERE type='table' AND name='host_groups'").Scan(&tableExists)

	// If hosts table exists but host_groups is missing, drop hosts table to perform schema upgrade
	if tableExists == 0 {
		var hostsExists int
		_ = db.conn.QueryRow("SELECT count(*) FROM sqlite_master WHERE type='table' AND name='hosts'").Scan(&hostsExists)
		if hostsExists > 0 {
			slog.Info("[DB] Migration required: recreating database tables to support groups schema.")
			_, _ = db.conn.Exec("DROP TABLE IF EXISTS hosts")
		}
	}

	// Create tables
	queryHosts := `
	CREATE TABLE IF NOT EXISTS hosts (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		host TEXT UNIQUE NOT NULL,
		name TEXT NOT NULL,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);`

	queryGroups := `
	CREATE TABLE IF NOT EXISTS groups (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		name TEXT UNIQUE NOT NULL,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);`

	queryHostGroups := `
	CREATE TABLE IF NOT EXISTS host_groups (
		host_id INTEGER,
		group_id INTEGER,
		PRIMARY KEY (host_id, group_id),
		FOREIGN KEY (host_id) REFERENCES hosts(id) ON DELETE CASCADE,
		FOREIGN KEY (group_id) REFERENCES groups(id) ON DELETE CASCADE
	);`

	if _, err := db.conn.Exec(queryHosts); err != nil {
		return fmt.Errorf("failed to create hosts table: %w", err)
	}
	if _, err := db.conn.Exec(queryGroups); err != nil {
		return fmt.Errorf("failed to create groups table: %w", err)
	}
	if _, err := db.conn.Exec(queryHostGroups); err != nil {
		return fmt.Errorf("failed to create host_groups table: %w", err)
	}

	return nil
}

// SeedDefaultHosts populates the database with default hosts, friendly names, and groups if empty.
func (db *DB) SeedDefaultHosts(defaultHosts []string) error {
	hosts, err := db.GetHosts()
	if err != nil {
		return err
	}
	if len(hosts) > 0 {
		return nil // Already has targets
	}

	// Predefined names & default groups
	nameMap := map[string]string{
		"google.com":     "Google Search",
		"github.com":     "GitHub",
		"cloudflare.com": "Cloudflare CDN",
		"8.8.8.8":        "Google DNS",
	}

	groupsMap := map[string][]string{
		"google.com":     {"web", "public"},
		"github.com":     {"web", "infrastructure"},
		"cloudflare.com": {"web"},
		"8.8.8.8":        {"dns", "public"},
	}

	slog.Info("[DB] Database is empty. Seeding default targets with groups", "targets", defaultHosts)
	for _, h := range defaultHosts {
		name, ok := nameMap[h]
		if !ok {
			name = h
		}
		grps := groupsMap[h]
		if len(grps) == 0 {
			grps = []string{"default"}
		}
		if err := db.AddHost(h, name, grps); err != nil {
			slog.Warn("[DB] Failed to seed default target", "host", h, "error", err)
		}
	}
	return nil
}

// AddHost inserts/updates a host, registers new groups, and links them.
func (db *DB) AddHost(host, name string, groups []string) error {
	host = strings.TrimSpace(host)
	name = strings.TrimSpace(name)
	if host == "" {
		return fmt.Errorf("host address cannot be empty")
	}
	if name == "" {
		name = host
	}

	tx, err := db.conn.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	// Insert or update host friendly name
	_, err = tx.Exec("INSERT INTO hosts (host, name) VALUES (?, ?) ON CONFLICT(host) DO UPDATE SET name=?", host, name, name)
	if err != nil {
		return fmt.Errorf("failed to insert/update host row: %w", err)
	}

	var hostID int64
	err = tx.QueryRow("SELECT id FROM hosts WHERE host = ?", host).Scan(&hostID)
	if err != nil {
		return fmt.Errorf("failed to retrieve host ID: %w", err)
	}

	// Remove existing group links for this host
	_, err = tx.Exec("DELETE FROM host_groups WHERE host_id = ?", hostID)
	if err != nil {
		return fmt.Errorf("failed to clean old host groups: %w", err)
	}

	// Link new groups
	for _, gName := range groups {
		gName = strings.TrimSpace(gName)
		if gName == "" {
			continue
		}

		// Insert group if it does not exist
		_, err = tx.Exec("INSERT OR IGNORE INTO groups (name) VALUES (?)", gName)
		if err != nil {
			return fmt.Errorf("failed to insert group %q: %w", gName, err)
		}

		var groupID int64
		err = tx.QueryRow("SELECT id FROM groups WHERE name = ?", gName).Scan(&groupID)
		if err != nil {
			return fmt.Errorf("failed to retrieve group ID: %w", err)
		}

		// Link host to group
		_, err = tx.Exec("INSERT OR IGNORE INTO host_groups (host_id, group_id) VALUES (?, ?)", hostID, groupID)
		if err != nil {
			return fmt.Errorf("failed to link host to group: %w", err)
		}
	}

	return tx.Commit()
}

// UpdateHost modifies a host's address, name, and updates its group links.
func (db *DB) UpdateHost(oldHost, newHost, name string, groups []string) error {
	oldHost = strings.TrimSpace(oldHost)
	newHost = strings.TrimSpace(newHost)
	name = strings.TrimSpace(name)
	if oldHost == "" || newHost == "" {
		return fmt.Errorf("host address cannot be empty")
	}
	if name == "" {
		name = newHost
	}

	tx, err := db.conn.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	// Update host address and name
	_, err = tx.Exec("UPDATE hosts SET host = ?, name = ? WHERE host = ?", newHost, name, oldHost)
	if err != nil {
		return fmt.Errorf("failed to update host row: %w", err)
	}

	var hostID int64
	err = tx.QueryRow("SELECT id FROM hosts WHERE host = ?", newHost).Scan(&hostID)
	if err != nil {
		return fmt.Errorf("failed to retrieve host ID: %w", err)
	}

	// Remove existing group links for this host
	_, err = tx.Exec("DELETE FROM host_groups WHERE host_id = ?", hostID)
	if err != nil {
		return fmt.Errorf("failed to clean old host groups: %w", err)
	}

	// Link new groups
	for _, gName := range groups {
		gName = strings.TrimSpace(gName)
		if gName == "" {
			continue
		}

		// Insert group if it does not exist
		_, err = tx.Exec("INSERT OR IGNORE INTO groups (name) VALUES (?)", gName)
		if err != nil {
			return fmt.Errorf("failed to insert group %q: %w", gName, err)
		}

		var groupID int64
		err = tx.QueryRow("SELECT id FROM groups WHERE name = ?", gName).Scan(&groupID)
		if err != nil {
			return fmt.Errorf("failed to retrieve group ID: %w", err)
		}

		// Link host to group
		_, err = tx.Exec("INSERT OR IGNORE INTO host_groups (host_id, group_id) VALUES (?, ?)", hostID, groupID)
		if err != nil {
			return fmt.Errorf("failed to link host to group: %w", err)
		}
	}

	return tx.Commit()
}

// DeleteHost deletes a host from the database (foreign keys handle host_groups cleanup).
func (db *DB) DeleteHost(host string) error {
	_, err := db.conn.Exec("DELETE FROM hosts WHERE host = ?", host)
	if err != nil {
		return fmt.Errorf("failed to delete host: %w", err)
	}
	return nil
}

// GetHosts returns all monitored hosts sorted alphabetically.
func (db *DB) GetHosts() ([]TargetHost, error) {
	return db.GetHostsFiltered("")
}

// GetHostsFiltered returns monitored hosts, optionally filtered by a group name.
func (db *DB) GetHostsFiltered(groupFilter string) ([]TargetHost, error) {
	var rows *sql.Rows
	var err error

	groupFilter = strings.TrimSpace(groupFilter)

	if groupFilter != "" {
		query := `
		SELECT h.host, h.name, GROUP_CONCAT(g2.name, ',') AS groups
		FROM hosts h
		JOIN host_groups hg ON h.id = hg.host_id
		JOIN groups g ON hg.group_id = g.id
		LEFT JOIN host_groups hg2 ON h.id = hg2.host_id
		LEFT JOIN groups g2 ON hg2.group_id = g2.id
		WHERE g.name = ?
		GROUP BY h.id
		ORDER BY h.host ASC;`
		rows, err = db.conn.Query(query, groupFilter)
	} else {
		query := `
		SELECT h.host, h.name, GROUP_CONCAT(g.name, ',') AS groups
		FROM hosts h
		LEFT JOIN host_groups hg ON h.id = hg.host_id
		LEFT JOIN groups g ON hg.group_id = g.id
		GROUP BY h.id
		ORDER BY h.host ASC;`
		rows, err = db.conn.Query(query)
	}

	if err != nil {
		return nil, fmt.Errorf("failed to query filtered hosts: %w", err)
	}
	defer rows.Close()

	var hosts []TargetHost
	for rows.Next() {
		var h TargetHost
		var groupConcat sql.NullString
		if err := rows.Scan(&h.Host, &h.Name, &groupConcat); err != nil {
			return nil, fmt.Errorf("failed to scan host row: %w", err)
		}

		if groupConcat.Valid {
			for _, grp := range strings.Split(groupConcat.String, ",") {
				grp = strings.TrimSpace(grp)
				if grp != "" {
					h.Groups = append(h.Groups, grp)
				}
			}
		}
		hosts = append(hosts, h)
	}

	return hosts, nil
}

// GetGroups returns all unique group names sorted alphabetically.
func (db *DB) GetGroups() ([]string, error) {
	rows, err := db.conn.Query("SELECT name FROM groups ORDER BY name ASC")
	if err != nil {
		return nil, fmt.Errorf("failed to query groups: %w", err)
	}
	defer rows.Close()

	var groups []string
	for rows.Next() {
		var gName string
		if err := rows.Scan(&gName); err != nil {
			return nil, fmt.Errorf("failed to scan group row: %w", err)
		}
		groups = append(groups, gName)
	}
	return groups, nil
}

// GroupInfo holds group statistics for UI management.
type GroupInfo struct {
	Name      string
	HostCount int
}

// CreateGroup inserts a new group into the database.
func (db *DB) CreateGroup(name string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return fmt.Errorf("group name cannot be empty")
	}
	_, err := db.conn.Exec("INSERT OR IGNORE INTO groups (name) VALUES (?)", name)
	if err != nil {
		return fmt.Errorf("failed to create group: %w", err)
	}
	return nil
}

// DeleteGroup deletes a group from the database.
func (db *DB) DeleteGroup(name string) error {
	_, err := db.conn.Exec("DELETE FROM groups WHERE name = ?", name)
	if err != nil {
		return fmt.Errorf("failed to delete group: %w", err)
	}
	return nil
}

// GetGroupsInfo returns all groups and the count of hosts associated with each.
func (db *DB) GetGroupsInfo() ([]GroupInfo, error) {
	query := `
	SELECT g.name, COUNT(hg.host_id) AS host_count
	FROM groups g
	LEFT JOIN host_groups hg ON g.id = hg.group_id
	GROUP BY g.id
	ORDER BY g.name ASC;`
	rows, err := db.conn.Query(query)
	if err != nil {
		return nil, fmt.Errorf("failed to query group info: %w", err)
	}
	defer rows.Close()

	var list []GroupInfo
	for rows.Next() {
		var gi GroupInfo
		if err := rows.Scan(&gi.Name, &gi.HostCount); err != nil {
			return nil, fmt.Errorf("failed to scan group info: %w", err)
		}
		list = append(list, gi)
	}
	return list, nil
}
