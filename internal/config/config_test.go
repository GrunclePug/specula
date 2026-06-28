package config

import (
	"os"
	"testing"
)

func TestLoad(t *testing.T) {
	// Set test environment variables
	os.Setenv("SPECULA_ADDR", ":9999")
	os.Setenv("SPECULA_PING_INTERVAL", "1s")
	os.Setenv("SPECULA_PING_TIMEOUT", "500ms")
	os.Setenv("SPECULA_TARGETS", "example.com,test.com")

	defer func() {
		os.Unsetenv("SPECULA_ADDR")
		os.Unsetenv("SPECULA_PING_INTERVAL")
		os.Unsetenv("SPECULA_PING_TIMEOUT")
		os.Unsetenv("SPECULA_TARGETS")
	}()

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Expected no error loading configuration, got: %v", err)
	}

	if cfg.Addr != ":9999" {
		t.Errorf("Expected Addr to be ':9999', got: %q", cfg.Addr)
	}

	if len(cfg.Targets) != 2 || cfg.Targets[0] != "example.com" || cfg.Targets[1] != "test.com" {
		t.Errorf("Expected targets [example.com, test.com], got: %v", cfg.Targets)
	}
}
