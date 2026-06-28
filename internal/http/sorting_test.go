package http

import "testing"

func TestCompareIPs(t *testing.T) {
	tests := []struct {
		h1, h2   string
		expected int // -1 means h1 < h2, 1 means h1 > h2, 0 means equal
	}{
		// Natural IP order checks
		{"192.168.1.2", "192.168.1.10", -1},
		{"192.168.1.10", "192.168.1.2", 1},
		{"192.168.1.2", "192.168.1.2", 0},
		
		// IP addresses vs Hostnames (IPs sort first)
		{"192.168.1.100", "google.com", -1},
		{"google.com", "192.168.1.100", 1},

		// Hostnames alphabetically
		{"cloudflare.com", "google.com", -1},
		{"google.com", "cloudflare.com", 1},
		{"google.com", "google.com", 0},

		// Ports ignored in comparison
		{"127.0.0.1:8080", "127.0.0.2:9000", -1},
	}

	for _, tt := range tests {
		res := compareIPs(tt.h1, tt.h2)
		// Normalize to -1, 0, or 1
		if res < 0 {
			res = -1
		} else if res > 0 {
			res = 1
		}
		if res != tt.expected {
			t.Errorf("compareIPs(%q, %q) = %d; expected %d", tt.h1, tt.h2, res, tt.expected)
		}
	}
}

func TestCompareNames(t *testing.T) {
	tests := []struct {
		n1, n2, h1, h2 string
		expected       int
	}{
		// Alphabetical names
		{"Alpha", "Beta", "10.0.0.1", "10.0.0.2", -1},
		{"Beta", "Alpha", "10.0.0.1", "10.0.0.2", 1},

		// Identical names -> fallback to natural IP sorting for stability
		{"Same", "Same", "192.168.1.2", "192.168.1.10", -1},
	}

	for _, tt := range tests {
		res := compareNames(tt.n1, tt.n2, tt.h1, tt.h2)
		if res < 0 {
			res = -1
		} else if res > 0 {
			res = 1
		}
		if res != tt.expected {
			t.Errorf("compareNames(%q, %q, %q, %q) = %d; expected %d", tt.n1, tt.n2, tt.h1, tt.h2, res, tt.expected)
		}
	}
}
