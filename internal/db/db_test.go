package db

import (
	"os"
	"testing"
)

func TestDBOperations(t *testing.T) {
	testDBPath := "test_specula.db"
	// Ensure cleanup
	defer func() {
		_ = os.Remove(testDBPath)
	}()

	// 1. Initialize
	database, err := New(testDBPath)
	if err != nil {
		t.Fatalf("Failed to initialize test DB: %v", err)
	}

	// 2. Add Target with groups
	if err := database.AddHost("8.8.8.8", "Google DNS", []string{"dns", "public"}); err != nil {
		t.Fatalf("Failed to add target: %v", err)
	}

	// 3. Add Duplicate (should update friendly name and reset groups list)
	if err := database.AddHost("8.8.8.8", "Google DNS Alternate", []string{"public"}); err != nil {
		t.Fatalf("Failed to add duplicate target: %v", err)
	}

	// 4. Retrieve targets
	hosts, err := database.GetHosts()
	if err != nil {
		t.Fatalf("Failed to query targets: %v", err)
	}
	if len(hosts) != 1 || hosts[0].Host != "8.8.8.8" || hosts[0].Name != "Google DNS Alternate" {
		t.Errorf("Unexpected targets list: %v", hosts)
	}
	if len(hosts[0].Groups) != 1 || hosts[0].Groups[0] != "public" {
		t.Errorf("Unexpected target groups: %v", hosts[0].Groups)
	}

	// 5. Filter by group
	filtered, err := database.GetHostsFiltered("public")
	if err != nil {
		t.Fatalf("Failed to query filtered targets: %v", err)
	}
	if len(filtered) != 1 {
		t.Errorf("Expected 1 filtered target, got: %d", len(filtered))
	}

	emptyFiltered, err := database.GetHostsFiltered("nonexistent")
	if err != nil {
		t.Fatalf("Failed to query nonexistent group: %v", err)
	}
	if len(emptyFiltered) != 0 {
		t.Errorf("Expected 0 filtered targets, got: %d", len(emptyFiltered))
	}

	// 5b. Update Target (change address from 8.8.8.8 to 8.8.4.4, change name, change groups)
	if err := database.UpdateHost("8.8.8.8", "8.8.4.4", "Google DNS Alternate Two", []string{"dns", "public"}); err != nil {
		t.Fatalf("Failed to update target: %v", err)
	}

	// Verify update changes
	hosts, err = database.GetHosts()
	if err != nil {
		t.Fatalf("Failed to query targets after update: %v", err)
	}
	if len(hosts) != 1 || hosts[0].Host != "8.8.4.4" || hosts[0].Name != "Google DNS Alternate Two" {
		t.Errorf("Unexpected updated target list: %v", hosts)
	}
	if len(hosts[0].Groups) != 2 || hosts[0].Groups[0] != "dns" || hosts[0].Groups[1] != "public" {
		t.Errorf("Unexpected updated groups list: %v", hosts[0].Groups)
	}

	// 6. Delete Target
	if err := database.DeleteHost("8.8.4.4"); err != nil {
		t.Fatalf("Failed to delete target: %v", err)
	}

	// 7. Verify deletion
	hosts, err = database.GetHosts()
	if err != nil {
		t.Fatalf("Failed to query targets after delete: %v", err)
	}
	if len(hosts) != 0 {
		t.Errorf("Expected empty targets list, got: %v", hosts)
	}

	// Close database
	if err := database.Close(); err != nil {
		t.Fatalf("Failed to close test database: %v", err)
	}
}
