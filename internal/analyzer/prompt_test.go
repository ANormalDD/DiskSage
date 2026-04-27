package analyzer

import (
	"strings"
	"testing"

	"disksage/internal/models"
)

func TestBuildPromptRequiresIntermediateStateOutsideReasoning(t *testing.T) {
	system, _ := BuildPrompt("tree", models.LLMConfig{})
	if !strings.Contains(system, "禁止把分析计划、候选目录列表或阶段性结论只放在推理") {
		t.Fatalf("expected system prompt to require intermediate state outside reasoning, got: %s", system)
	}
}
