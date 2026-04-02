package config

import (
	"path/filepath"
	"testing"

	"disksage/internal/models"
)

func TestConfigLoadSave(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cfg.json")
	m := NewManager(path)

	cfg := models.DefaultAppConfig()
	cfg.LLM.Model = "gpt-4o-mini"
	if err := m.Save(cfg); err != nil {
		t.Fatalf("save failed: %v", err)
	}

	loaded, err := m.Load()
	if err != nil {
		t.Fatalf("load failed: %v", err)
	}
	if loaded.LLM.Model != cfg.LLM.Model {
		t.Fatalf("unexpected model: %s", loaded.LLM.Model)
	}
}
