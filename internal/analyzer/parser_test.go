package analyzer

import (
	"testing"

	"disksage/internal/models"
)

func TestParseRecommendations(t *testing.T) {
	raw := `{"recommendations":[{"path":"D:/temp","size":1000,"category":"safe","reason":"x","clean_method":"recycle","command":"","risk":"low"}]}`
	rows, err := ParseRecommendations(raw)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected 1 recommendation")
	}
	if rows[0].Path != "D:/temp" {
		t.Fatalf("unexpected path: %s", rows[0].Path)
	}
}

func TestParseRecommendationsFromReasoningText(t *testing.T) {
	raw := `（我来分析这个目录树，先给出建议）
以下是分析：
[{
  "path":"D:/temp",
  "size":2048,
  "category":"safe",
  "reason":"temp",
  "clean_method":"recycle",
  "command":"",
  "risk":"low"
}]`
	rows, err := ParseRecommendations(raw)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected 1 recommendation")
	}
	if rows[0].Path != "D:/temp" {
		t.Fatalf("unexpected path: %s", rows[0].Path)
	}
}

func TestSanitizeRecommendationsGuardsBroadMixedDirectory(t *testing.T) {
	in := []models.Recommendation{
		{
			Path:        `C:\Users\wang\AppData\Roaming`,
			Size:        123,
			Category:    models.CategorySafe,
			Reason:      "应用程序缓存和dump文件，可清理部分缓存",
			CleanMethod: models.MethodRecycle,
			Risk:        "低风险",
		},
	}

	out := sanitizeRecommendations(in)
	if len(out) != 1 {
		t.Fatalf("expected 1 recommendation, got %d", len(out))
	}
	if out[0].Category != models.CategoryReview {
		t.Fatalf("expected category review, got %s", out[0].Category)
	}
	if out[0].CleanMethod != models.MethodRedirect {
		t.Fatalf("expected clean method redirect, got %s", out[0].CleanMethod)
	}
}

func TestSanitizeRecommendationsKeepsSpecificCacheSubdir(t *testing.T) {
	in := []models.Recommendation{
		{
			Path:        `C:\Users\wang\AppData\Roaming\clash-verge\logs`,
			Size:        123,
			Category:    models.CategorySafe,
			Reason:      "日志可清理",
			CleanMethod: models.MethodRecycle,
			Risk:        "低风险",
		},
	}

	out := sanitizeRecommendations(in)
	if len(out) != 1 {
		t.Fatalf("expected 1 recommendation, got %d", len(out))
	}
	if out[0].Category != models.CategorySafe {
		t.Fatalf("expected category safe, got %s", out[0].Category)
	}
	if out[0].CleanMethod != models.MethodRecycle {
		t.Fatalf("expected clean method recycle, got %s", out[0].CleanMethod)
	}
}
