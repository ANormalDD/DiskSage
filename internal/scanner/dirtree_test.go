package scanner

import (
	"strings"
	"testing"

	"disksage/internal/models"
)

func TestRenderCompressedTree(t *testing.T) {
	root := models.DirNode{
		Path: "D:/",
		Children: []models.DirNode{
			{Name: "A", Size: 1000},
			{Name: "B", Size: 900},
			{Name: "C", Size: 10},
		},
	}
	out := RenderCompressedTree(root, RenderConfig{TopNPerLevel: 1, MinChildSize: 100})
	if !strings.Contains(out, "A/") {
		t.Fatalf("expected top dir in output, got: %s", out)
	}
	if !strings.Contains(out, "more dirs/files") {
		t.Fatalf("expected aggregated remainder")
	}
}
