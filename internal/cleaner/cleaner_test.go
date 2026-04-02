package cleaner

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"disksage/internal/models"
)

func TestCleanerDelete(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "to-delete")
	if err := os.MkdirAll(target, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(target, "x.bin"), make([]byte, 64), 0o644); err != nil {
		t.Fatal(err)
	}

	c := NewCleaner(Options{HistoryPath: filepath.Join(root, "history.jsonl")})
	summary, err := c.Clean(context.Background(), models.CleanRequest{
		Items: []models.Recommendation{{
			Path:        target,
			Size:        64,
			Category:    models.CategorySafe,
			CleanMethod: models.MethodDelete,
		}},
		PermanentDelete: true,
		RequestedBy:     "test",
	})
	if err != nil {
		t.Fatalf("clean failed: %v", err)
	}
	if summary.Freed != 64 {
		t.Fatalf("unexpected freed size: %d", summary.Freed)
	}
	if _, statErr := os.Stat(target); !os.IsNotExist(statErr) {
		t.Fatalf("target should be deleted")
	}
}

func TestCleanerCommandDenied(t *testing.T) {
	c := NewCleaner(Options{HistoryPath: filepath.Join(t.TempDir(), "history.jsonl")})
	summary, err := c.Clean(context.Background(), models.CleanRequest{
		Items: []models.Recommendation{{
			Path:        "D:/any",
			Size:        1,
			Category:    models.CategoryManual,
			CleanMethod: models.MethodCommand,
			Command:     "rm -rf /",
		}},
		ConfirmCommands: true,
	})
	if err != nil {
		t.Fatalf("clean should not return global error: %v", err)
	}
	if len(summary.Results) != 1 || summary.Results[0].Success {
		t.Fatalf("command should fail at item level")
	}
}
