package config

import (
	"os"
	"path/filepath"
	"testing"
)

// TestIncludeFilesMerge verifies that local.include_files and
// forwarder.include_files glob patterns are merged into the parent config's
// static records / forwarder rules on load.
func TestIncludeFilesMerge(t *testing.T) {
	dir := t.TempDir()

	mainYAML := `
server:
  listen:
    - 127.0.0.1:5353
upstream:
  addresses:
    - 1.1.1.1:53
local:
  enable: true
  static_records:
    - domain: direct.example.com
      ip: 10.0.0.1
  include_files:
    - conf.d/local-*.yaml
forwarder:
  enable: true
  rules:
    - domain: direct.example.com
      upstreams:
        - 10.0.0.53:53
  include_files:
    - conf.d/forwarder-*.yaml
`
	if err := os.WriteFile(filepath.Join(dir, "config.yaml"), []byte(mainYAML), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := os.Mkdir(filepath.Join(dir, "conf.d"), 0o755); err != nil {
		t.Fatal(err)
	}

	localIncludeYAML := `
static_records:
  - domain: included.example.com
    ip: 10.0.0.2
`
	if err := os.WriteFile(filepath.Join(dir, "conf.d", "local-extra.yaml"), []byte(localIncludeYAML), 0o644); err != nil {
		t.Fatal(err)
	}

	forwarderIncludeYAML := `
rules:
  - domain: included.example.com
    upstreams:
      - 10.0.0.54:53
`
	if err := os.WriteFile(filepath.Join(dir, "conf.d", "forwarder-extra.yaml"), []byte(forwarderIncludeYAML), 0o644); err != nil {
		t.Fatal(err)
	}

	mgr, err := NewManager(filepath.Join(dir, "config.yaml"))
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}

	cfg := mgr.GetConfig()

	if got := len(cfg.Local.StaticRecords); got != 2 {
		t.Fatalf("expected 2 static records (1 direct + 1 included), got %d: %+v", got, cfg.Local.StaticRecords)
	}

	if got := len(cfg.Forwarder.Rules); got != 2 {
		t.Fatalf("expected 2 forwarder rules (1 direct + 1 included), got %d: %+v", got, cfg.Forwarder.Rules)
	}

	foundIncludedRecord := false
	for _, r := range cfg.Local.StaticRecords {
		if r.Domain == "included.example.com" && r.IP == "10.0.0.2" {
			foundIncludedRecord = true
		}
	}
	if !foundIncludedRecord {
		t.Fatalf("included static record not merged: %+v", cfg.Local.StaticRecords)
	}

	foundIncludedRule := false
	for _, r := range cfg.Forwarder.Rules {
		if r.Domain == "included.example.com" && len(r.Upstreams) == 1 && r.Upstreams[0] == "10.0.0.54:53" {
			foundIncludedRule = true
		}
	}
	if !foundIncludedRule {
		t.Fatalf("included forwarder rule not merged: %+v", cfg.Forwarder.Rules)
	}
}
