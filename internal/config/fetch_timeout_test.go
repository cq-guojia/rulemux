package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func writeTimeoutCfg(t *testing.T, body string) *Config {
	t.Helper()
	p := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	c, err := Load(p)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	return c
}

func TestFetchTimeoutConfigured(t *testing.T) {
	c := writeTimeoutCfg(t, "fetch_timeout = \"5s\"\n[[source]]\npath = \"a.md\"\n")
	if got := c.FetchTimeoutDuration(); got != 5*time.Second {
		t.Errorf("FetchTimeoutDuration = %v, want 5s", got)
	}
}

func TestFetchTimeoutDefaultAndInvalid(t *testing.T) {
	if got := writeTimeoutCfg(t, "[[source]]\npath = \"a.md\"\n").FetchTimeoutDuration(); got != DefaultFetchTimeout {
		t.Errorf("absent fetch_timeout = %v, want default %v", got, DefaultFetchTimeout)
	}
	if got := writeTimeoutCfg(t, "fetch_timeout = \"not-a-duration\"\n[[source]]\npath = \"a.md\"\n").FetchTimeoutDuration(); got != DefaultFetchTimeout {
		t.Errorf("invalid fetch_timeout = %v, want default %v", got, DefaultFetchTimeout)
	}
}
