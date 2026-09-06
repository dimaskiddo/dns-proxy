package config

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/spf13/viper"
	"gopkg.in/yaml.v3"

	"github.com/dimaskiddo/dns-proxy/internal/util"
)

// Manager loads and hot-reloads the YAML configuration file, merging any
// local/forwarder include_files glob patterns on every load.
type Manager struct {
	mu     sync.RWMutex
	file   string
	config *Config
}

// NewManager loads configFile and returns a ready Manager.
func NewManager(configFile string) (*Manager, error) {
	m := &Manager{file: configFile}

	if err := m.Reload(); err != nil {
		return nil, err
	}

	return m, nil
}

// Reload re-reads the config file from disk, replacing the current config
// only if the new one loads and validates successfully.
func (m *Manager) Reload() error {
	cfg := &Config{}
	setDefaultConfig(cfg)

	v := viper.New()
	v.SetConfigFile(m.file)
	v.SetConfigType("yaml")

	if err := v.ReadInConfig(); err != nil {
		return fmt.Errorf("failed to load configuration '%s': %w", m.file, err)
	}

	if err := v.Unmarshal(cfg); err != nil {
		return fmt.Errorf("failed to unmarshal configuration: %w", err)
	}

	if len(cfg.Server.Listen) == 0 {
		return fmt.Errorf("no listen addresses configured")
	}

	if err := mergeIncludeFiles(filepath.Dir(m.file), cfg); err != nil {
		return err
	}

	m.mu.Lock()
	m.config = cfg
	m.mu.Unlock()

	return nil
}

// GetConfig returns the currently loaded configuration.
func (m *Manager) GetConfig() *Config {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return m.config
}

// mergeIncludeFiles expands cfg.Local.IncludeFiles / cfg.Forwarder.IncludeFiles
// glob patterns (relative to configDir) and appends their static
// records/forwarder rules onto cfg. Unreadable or unparsable include files
// are skipped, matching the original behavior.
func mergeIncludeFiles(configDir string, cfg *Config) error {
	for _, file := range util.ParseIncludeFiles(configDir, cfg.Local.IncludeFiles) {
		data, err := os.ReadFile(file)
		if err != nil {
			continue
		}

		var included LocalConfig
		if err := yaml.Unmarshal(data, &included); err == nil {
			if len(included.StaticRecords) > 0 {
				cfg.Local.StaticRecords = append(cfg.Local.StaticRecords, included.StaticRecords...)
			}
		}
	}

	for _, file := range util.ParseIncludeFiles(configDir, cfg.Forwarder.IncludeFiles) {
		data, err := os.ReadFile(file)
		if err != nil {
			continue
		}

		var included ForwarderConfig
		if err := yaml.Unmarshal(data, &included); err == nil {
			if len(included.Rules) > 0 {
				cfg.Forwarder.Rules = append(cfg.Forwarder.Rules, included.Rules...)
			}
		}
	}

	return nil
}
