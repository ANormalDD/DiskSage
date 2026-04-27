package analyzer

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"

	"disksage/internal/models"
)

func ParseRecommendations(raw string) ([]models.Recommendation, error) {
	payload := preprocessModelText(raw)

	if payload == "" {
		return nil, fmt.Errorf("empty recommendation payload")
	}

	candidate := payload
	if extracted, ok := extractJSONCandidate(payload); ok {
		candidate = extracted
	}

	var list []models.Recommendation
	if err := json.Unmarshal([]byte(candidate), &list); err != nil {
		var wrapped struct {
			Recommendations []models.Recommendation `json:"recommendations"`
		}
		if err2 := json.Unmarshal([]byte(candidate), &wrapped); err2 != nil {
			return nil, fmt.Errorf("parse recommendations failed: %w", err)
		}
		list = wrapped.Recommendations
	}
	return sanitizeRecommendations(list), nil
}

func preprocessModelText(raw string) string {
	payload := strings.TrimSpace(raw)
	payload = strings.TrimPrefix(payload, "```json")
	payload = strings.TrimPrefix(payload, "```")
	payload = strings.TrimSuffix(payload, "```")
	payload = strings.TrimSpace(payload)

	thinkTag := regexp.MustCompile(`(?s)<think>.*?</think>`)
	payload = thinkTag.ReplaceAllString(payload, "")

	return strings.TrimSpace(payload)
}

func extractJSONCandidate(text string) (string, bool) {
	runes := []rune(text)
	start := -1
	for i, r := range runes {
		if r == '[' || r == '{' {
			start = i
			break
		}
	}
	if start == -1 {
		return "", false
	}

	for end := len(runes); end > start; end-- {
		candidate := strings.TrimSpace(string(runes[start:end]))
		if !strings.HasSuffix(candidate, "]") && !strings.HasSuffix(candidate, "}") {
			continue
		}
		var anyJSON any
		if json.Unmarshal([]byte(candidate), &anyJSON) == nil {
			return candidate, true
		}
	}

	return "", false
}

func sanitizeRecommendations(in []models.Recommendation) []models.Recommendation {
	seen := make(map[string]struct{}, len(in))
	out := make([]models.Recommendation, 0, len(in))
	for _, item := range in {
		item.Path = strings.TrimSpace(item.Path)
		if item.Path == "" {
			continue
		}
		// Reject wildcard paths (e.g. C:\Users\foo\*.log) – the backend cannot
		// measure or delete glob patterns; the LLM must submit a concrete directory.
		if strings.ContainsAny(item.Path, "*?") {
			continue
		}
		if _, ok := seen[item.Path]; ok {
			continue
		}
		// Always zero the LLM-provided size; the backend measures it via resolveRecommendationSizes.
		item.Size = 0
		if item.CleanMethod == "" {
			item.CleanMethod = models.MethodRecycle
		}
		if item.Category == "" {
			item.Category = models.CategoryReview
		}
		item = enforcePartialCleanupGuard(item)
		seen[item.Path] = struct{}{}
		out = append(out, item)
	}
	return out
}

func enforcePartialCleanupGuard(item models.Recommendation) models.Recommendation {
	if !shouldForceSpecificSubpath(item.Path, item.Reason) {
		return item
	}

	item.Category = models.CategoryReview
	item.CleanMethod = models.MethodRedirect
	item.Command = ""

	if strings.TrimSpace(item.Reason) == "" {
		item.Reason = "目录可能仅部分可清理，建议继续下探到具体缓存/日志子目录"
	} else if !strings.Contains(item.Reason, "仅部分可清理") {
		item.Reason = item.Reason + "；仅部分可清理，需继续下探"
	}

	if strings.TrimSpace(item.Risk) == "" {
		item.Risk = "整目录清理风险高，建议仅清理明确的缓存/日志子目录"
	} else if !strings.Contains(item.Risk, "整目录") {
		item.Risk = item.Risk + "；整目录清理风险高"
	}

	return item
}

func shouldForceSpecificSubpath(path string, reason string) bool {
	norm := strings.ToLower(filepath.ToSlash(filepath.Clean(path)))
	if norm == "" || norm == "." {
		return false
	}
	if hasDisposableToken(norm) {
		return false
	}

	if strings.HasSuffix(norm, "/appdata") || strings.HasSuffix(norm, "/appdata/roaming") || strings.HasSuffix(norm, "/appdata/local") || strings.HasSuffix(norm, "/onedrive") {
		return true
	}

	segments := strings.Split(strings.Trim(norm, "/"), "/")
	for i := 0; i < len(segments)-1; i++ {
		if segments[i] == "appdata" && (segments[i+1] == "roaming" || segments[i+1] == "local") {
			trailing := len(segments) - (i + 2)
			if trailing <= 1 {
				return true
			}
		}
	}

	reasonLower := strings.ToLower(strings.TrimSpace(reason))
	if strings.Contains(reasonLower, "部分") || strings.Contains(reasonLower, "partial") {
		if filepath.Ext(norm) == "" {
			return true
		}
	}

	return false
}

func hasDisposableToken(normPath string) bool {
	tokens := []string{
		"/cache",
		"/caches",
		"/tmp",
		"/temp",
		"/logs",
		"/log",
		"/dump",
		"/dumps",
		".dmp",
		"/crash",
		"crashpad",
	}
	for _, token := range tokens {
		if strings.Contains(normPath, token) {
			return true
		}
	}
	return false
}
