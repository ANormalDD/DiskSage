package analyzer

import "testing"

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
