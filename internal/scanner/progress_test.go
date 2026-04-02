package scanner

import (
	"os"
	"path/filepath"
	"testing"

	"disksage/internal/models"
)

func TestScanProgressCallbackEmitsDone(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "a", "b"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "a", "b", "x.bin"), make([]byte, 64), 0o644); err != nil {
		t.Fatal(err)
	}

	s := NewScanner(models.ScanConfig{MaxDepth: 4, MinDirSize: 1, TopN: 20})
	events := make([]models.ScanProgress, 0, 8)
	s.SetProgressCallback(func(p models.ScanProgress) {
		events = append(events, p)
	})

	_, err := s.ScanDrive(root)
	if err != nil {
		t.Fatalf("scan failed: %v", err)
	}
	if len(events) == 0 {
		t.Fatalf("expected progress events")
	}
	last := events[len(events)-1]
	if !last.Done {
		t.Fatalf("expected last event done")
	}
	if last.DirsSeen == 0 {
		t.Fatalf("expected dirs seen > 0")
	}
}
