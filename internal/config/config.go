package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"

	"disksage/internal/models"
)

type Manager struct {
	path string
	mu   sync.RWMutex
}

func NewManager(path string) *Manager {
	if path == "" {
		path = defaultConfigPath()
	}
	return &Manager{path: path}
}

func (m *Manager) Load() (models.AppConfig, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	cfg := models.DefaultAppConfig()
	blob, err := os.ReadFile(m.path)
	if err != nil {
		if os.IsNotExist(err) {
			return cfg, nil
		}
		return cfg, err
	}
	if err := json.Unmarshal(blob, &cfg); err != nil {
		return models.DefaultAppConfig(), err
	}
	if err := cfg.Validate(); err != nil {
		return models.DefaultAppConfig(), err
	}
	return cfg, nil
}

func (m *Manager) Save(cfg models.AppConfig) error {
	if err := cfg.Validate(); err != nil {
		return err
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	if err := os.MkdirAll(filepath.Dir(m.path), 0o755); err != nil {
		return err
	}
	blob, _ := json.MarshalIndent(cfg, "", "  ")
	return os.WriteFile(m.path, blob, 0o644)
}

func defaultConfigPath() string {
	appData := os.Getenv("APPDATA")
	if appData == "" {
		appData = os.TempDir()
	}
	return filepath.Join(appData, "DiskSage", "config.json")
}
