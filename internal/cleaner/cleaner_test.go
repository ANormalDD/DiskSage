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

func TestCleanerRedirectUsesAutoDelete(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "redirect-delete")
	if err := os.MkdirAll(target, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(target, "x.bin"), make([]byte, 128), 0o644); err != nil {
		t.Fatal(err)
	}

	c := NewCleaner(Options{HistoryPath: filepath.Join(root, "history.jsonl")})
	summary, err := c.Clean(context.Background(), models.CleanRequest{
		Items: []models.Recommendation{{
			Path:        target,
			Size:        128,
			Category:    models.CategoryReview,
			CleanMethod: models.MethodRedirect,
		}},
		PermanentDelete: true,
		RequestedBy:     "test",
	})
	if err != nil {
		t.Fatalf("clean failed: %v", err)
	}
	if summary.Freed != 128 {
		t.Fatalf("unexpected freed size: %d", summary.Freed)
	}
	if _, statErr := os.Stat(target); !os.IsNotExist(statErr) {
		t.Fatalf("target should be deleted")
	}
}

func TestCleanerNonPermanentFlagIgnored(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "recycle-ignored")
	if err := os.MkdirAll(target, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(target, "x.bin"), make([]byte, 256), 0o644); err != nil {
		t.Fatal(err)
	}

	recycleRoot := filepath.Join(os.TempDir(), "disksage", "recycle")
	_ = os.RemoveAll(recycleRoot)

	c := NewCleaner(Options{HistoryPath: filepath.Join(root, "history.jsonl")})
	summary, err := c.Clean(context.Background(), models.CleanRequest{
		Items: []models.Recommendation{{
			Path:        target,
			Size:        256,
			Category:    models.CategorySafe,
			CleanMethod: models.MethodRecycle,
		}},
		PermanentDelete: false,
		RequestedBy:     "test",
	})
	if err != nil {
		t.Fatalf("clean failed: %v", err)
	}
	if summary.Freed != 256 {
		t.Fatalf("unexpected freed size: %d", summary.Freed)
	}
	if _, statErr := os.Stat(target); !os.IsNotExist(statErr) {
		t.Fatalf("target should be deleted")
	}
	if info, statErr := os.Stat(recycleRoot); statErr == nil {
		if !info.IsDir() {
			t.Fatalf("recycle path should be a directory when present")
		}
		entries, readErr := os.ReadDir(recycleRoot)
		if readErr != nil {
			t.Fatalf("read recycle dir failed: %v", readErr)
		}
		if len(entries) > 0 {
			t.Fatalf("non-permanent recycle staging should not be used")
		}
	} else if !os.IsNotExist(statErr) {
		t.Fatalf("stat recycle dir failed: %v", statErr)
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
