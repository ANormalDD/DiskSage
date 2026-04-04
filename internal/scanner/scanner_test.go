package scanner

import (
	"os"
	"path/filepath"
	"testing"

	"disksage/internal/models"
)

func TestScanDriveAndMarkers(t *testing.T) {
	root := t.TempDir()
	project := filepath.Join(root, "project")
	cache := filepath.Join(root, "temp")
	if err := os.MkdirAll(project, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(cache, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(project, "package.json"), []byte(`{"name":"demo"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cache, "a.log"), make([]byte, 1024), 0o644); err != nil {
		t.Fatal(err)
	}

	s := NewScanner(models.ScanConfig{MaxDepth: 4, MinDirSize: 1, TopN: 20})
	node, err := s.ScanDrive(root)
	if err != nil {
		t.Fatalf("scan failed: %v", err)
	}
	if node.Size <= 0 {
		t.Fatalf("expected size > 0")
	}

	foundMarker := false
	for _, c := range node.Children {
		for _, label := range c.MarkerLabels {
			if label == "nodejs" {
				foundMarker = true
			}
		}
	}
	if !foundMarker {
		t.Fatalf("expected nodejs marker")
	}
}

func TestCheckDirContent(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "a.zip"), make([]byte, 10), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "b.zip"), make([]byte, 10), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "c.iso"), make([]byte, 10), 0o644); err != nil {
		t.Fatal(err)
	}

	s := NewScanner(models.DefaultScanConfig())
	dist, err := s.CheckDirContent(root)
	if err != nil {
		t.Fatalf("CheckDirContent failed: %v", err)
	}
	if len(dist.Stats) == 0 {
		t.Fatalf("expected stats")
	}
	if dist.CreatedAt.IsZero() {
		t.Fatalf("expected created_at to be populated")
	}
	if dist.ModifiedAt.IsZero() {
		t.Fatalf("expected modified_at to be populated")
	}
}
