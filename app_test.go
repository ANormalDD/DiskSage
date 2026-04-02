package main

import (
	"os"
	"path/filepath"
	"testing"

	"disksage/internal/analyzer"
	"disksage/internal/models"
)

func TestAppCleanValidation(t *testing.T) {
	app := NewApp()
	_, err := app.Clean(models.CleanRequest{})
	if err == nil {
		t.Fatalf("expected validation error")
	}
}

func TestAppSaveConfigValidation(t *testing.T) {
	app := NewApp()
	cfg := models.DefaultAppConfig()
	cfg.LLM.Model = ""
	if err := app.SaveConfig(cfg); err == nil {
		t.Fatalf("expected invalid config error")
	}
}

func TestAppScanAndAnalyze(t *testing.T) {
	app := NewApp()
	app.analyzer.SetClient(analyzer.NewHeuristicClient())

	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "temp"), 0o755); err != nil {
		t.Fatal(err)
	}
	cachePath := filepath.Join(root, "temp", "cache.bin")
	f, err := os.Create(cachePath)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.Truncate(80 * 1024 * 1024); err != nil {
		_ = f.Close()
		t.Fatal(err)
	}
	_ = f.Close()

	res, err := app.ScanDrive(root)
	if err != nil {
		t.Fatalf("scan failed: %v", err)
	}
	if res.Compressed == "" {
		t.Fatalf("expected compressed tree")
	}

	recs, err := app.AnalyzeLastScan()
	if err != nil {
		t.Fatalf("analyze failed: %v", err)
	}
	if len(recs) == 0 {
		t.Fatalf("expected recommendations")
	}
}
