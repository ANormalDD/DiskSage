package analyzer

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"disksage/internal/models"
)

type sequenceClient struct {
	responses []LLMResponse
	errs      []error
	idx       int
}

func (c *sequenceClient) Complete(ctx context.Context, req LLMRequest) (LLMResponse, error) {
	_ = ctx
	_ = req
	if c.idx >= len(c.responses) {
		return LLMResponse{}, nil
	}
	resp := c.responses[c.idx]
	err := c.errs[c.idx]
	c.idx++
	return resp, err
}

type countingClient struct {
	calls         int
	succeedAtCall int
}

func (c *countingClient) Complete(ctx context.Context, req LLMRequest) (LLMResponse, error) {
	_ = ctx
	_ = req
	c.calls++
	if c.succeedAtCall > 0 && c.calls == c.succeedAtCall {
		return LLMResponse{
			Recommendations: []models.Recommendation{{
				Path:        "D:/temp",
				Size:        100,
				Category:    models.CategorySafe,
				Reason:      "ok",
				CleanMethod: models.MethodRecycle,
				Risk:        "low",
			}},
		}, nil
	}
	return LLMResponse{Content: "继续分析"}, nil
}

type interruptedStreamClient struct {
	calls       int
	capturedReq []LLMRequest
}

func (c *interruptedStreamClient) Complete(ctx context.Context, req LLMRequest) (LLMResponse, error) {
	_ = ctx
	c.calls++
	c.capturedReq = append(c.capturedReq, req)

	if c.calls == 1 {
		if req.OnStreamDelta != nil {
			req.OnStreamDelta(LLMStreamDelta{Reasoning: "先检查下载目录"})
			req.OnStreamDelta(LLMStreamDelta{Content: "已发现 Downloads 下有大文件"})
		}
		return LLMResponse{}, errors.New("context deadline exceeded")
	}

	return LLMResponse{
		Recommendations: []models.Recommendation{{
			Path:        "D:/Downloads/archive.iso",
			Size:        100,
			Category:    models.CategoryConfirm,
			Reason:      "large archive",
			CleanMethod: models.MethodRedirect,
			Risk:        "medium",
		}},
	}, nil
}

func TestHeuristicAnalyzer(t *testing.T) {
	a := NewAnalyzer(Options{Client: NewHeuristicClient()})
	root := models.DirNode{
		Path: "D:/",
		Children: []models.DirNode{
			{Path: "D:/temp", Name: "temp", Size: 600 * 1024 * 1024, ModTime: time.Now().AddDate(0, -2, 0)},
			{Path: "D:/Downloads", Name: "Downloads", Size: 800 * 1024 * 1024, ModTime: time.Now().AddDate(0, -8, 0)},
			{Path: "D:/project/node_modules", Name: "node_modules", Size: 900 * 1024 * 1024, ModTime: time.Now().AddDate(0, -6, 0)},
		},
	}
	recs, err := a.Analyze(context.Background(), root)
	if err != nil {
		t.Fatalf("analyze failed: %v", err)
	}
	if len(recs) < 2 {
		t.Fatalf("expected heuristic recommendations")
	}

	stats := a.GetTokenStats()
	if stats.RequestCount != 1 {
		t.Fatalf("expected request count 1, got %d", stats.RequestCount)
	}
	if stats.Last.TotalTokens <= 0 {
		t.Fatalf("expected token usage to be recorded")
	}

	debug := a.GetDebugInfo()
	if debug.Source == "" {
		t.Fatalf("expected debug source to be recorded")
	}
	if debug.UpdatedAt.IsZero() {
		t.Fatalf("expected debug updated timestamp")
	}
}

func TestAnalyzerRepairsReasoningContent(t *testing.T) {
	client := &sequenceClient{
		responses: []LLMResponse{
			{Content: "（我来分析这个目录树，先给出建议）", Usage: models.TokenUsage{InputTokens: 10, OutputTokens: 5, TotalTokens: 15}},
			{Content: `[{"path":"D:/temp","size":1234,"category":"safe","reason":"temp","clean_method":"recycle","command":"","risk":"low"}]`, Usage: models.TokenUsage{InputTokens: 3, OutputTokens: 7, TotalTokens: 10}},
		},
		errs: []error{nil, nil},
	}

	a := NewAnalyzer(Options{Client: client})
	root := models.DirNode{Path: "D:/", Size: 1}
	recs, err := a.Analyze(context.Background(), root)
	if err != nil {
		t.Fatalf("analyze failed: %v", err)
	}
	if len(recs) != 1 {
		t.Fatalf("expected repaired recommendations, got %d", len(recs))
	}
	if recs[0].Path != "D:/temp" {
		t.Fatalf("unexpected path: %s", recs[0].Path)
	}

	debug := a.GetDebugInfo()
	if debug.Source != "llm" {
		t.Fatalf("expected llm source, got %s", debug.Source)
	}
}

func TestAnalyzerAutoProbeAfterRepeatedNonCompliantTurns(t *testing.T) {
	client := &sequenceClient{
		responses: []LLMResponse{
			{Content: "我先分析一下目录结构。"},
			{Content: "我需要更多信息来判断。"},
			{Content: `[{"path":"D:/Games","size":5000,"category":"confirm","reason":"large","clean_method":"redirect","command":"","risk":"medium"}]`},
		},
		errs: []error{nil, nil, nil},
	}

	scanCalled := 0
	a := NewAnalyzer(Options{
		Client: client,
		ScanDeeper: func(path string, depth int) (models.DirNode, error) {
			scanCalled++
			return models.DirNode{Path: path, Name: "probe", Size: 12345}, nil
		},
	})

	root := models.DirNode{
		Path: "D:/",
		Children: []models.DirNode{
			{Path: "D:/Games", Name: "Games", Size: 2 * 1024 * 1024 * 1024},
			{Path: "D:/Downloads", Name: "Downloads", Size: 1 * 1024 * 1024 * 1024},
		},
	}

	recs, err := a.Analyze(context.Background(), root)
	if err != nil {
		t.Fatalf("analyze failed: %v", err)
	}
	if len(recs) != 1 {
		t.Fatalf("expected 1 recommendation, got %d", len(recs))
	}
	if scanCalled == 0 {
		t.Fatalf("expected auto probe to call scanDeeper")
	}
}

func TestAnalyzerRespectsConfiguredMaxTurns(t *testing.T) {
	client := &countingClient{}
	a := NewAnalyzer(Options{
		Client: client,
		Config: models.LLMConfig{MaxTurns: 2},
	})

	_, err := a.Analyze(context.Background(), models.DirNode{Path: "D:/", Size: 1})
	if err == nil {
		t.Fatalf("expected analyze to fail after max turns")
	}
	if client.calls != 2 {
		t.Fatalf("expected exactly 2 turns, got %d", client.calls)
	}
}

func TestAnalyzerUnlimitedTurnsWhenConfiguredMinusOne(t *testing.T) {
	client := &countingClient{succeedAtCall: 8}
	a := NewAnalyzer(Options{
		Client: client,
		Config: models.LLMConfig{MaxTurns: -1},
	})

	recs, err := a.Analyze(context.Background(), models.DirNode{Path: "D:/", Size: 1})
	if err != nil {
		t.Fatalf("expected analyze to succeed in unlimited mode: %v", err)
	}
	if len(recs) != 1 {
		t.Fatalf("expected 1 recommendation, got %d", len(recs))
	}
	if client.calls != 8 {
		t.Fatalf("expected to continue beyond default turns, got %d", client.calls)
	}
}

func TestAnalyzerUnlimitedModeStopsOnContextCancel(t *testing.T) {
	client := &countingClient{}
	a := NewAnalyzer(Options{
		Client: client,
		Config: models.LLMConfig{MaxTurns: -1},
	})

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := a.Analyze(ctx, models.DirNode{Path: "D:/", Size: 1})
	if err == nil {
		t.Fatalf("expected cancellation error")
	}
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context canceled error, got: %v", err)
	}
}

func TestAnalyzerCanContinueAfterRateLimitPause(t *testing.T) {
	client := &sequenceClient{
		responses: []LLMResponse{
			{},
			{
				Recommendations: []models.Recommendation{{
					Path:        "D:/temp",
					Size:        100,
					Category:    models.CategorySafe,
					Reason:      "ok",
					CleanMethod: models.MethodRecycle,
					Risk:        "low",
				}},
			},
		},
		errs: []error{errors.New("429 Too Many Requests"), nil},
	}

	a := NewAnalyzer(Options{Client: client})
	_, err := a.Analyze(context.Background(), models.DirNode{Path: "D:/", Size: 1})
	if !errors.Is(err, ErrRateLimitPaused) {
		t.Fatalf("expected rate limit pause error, got: %v", err)
	}
	if !a.HasPendingAnalysis() {
		t.Fatalf("expected pending analysis after rate limit pause")
	}

	recs, err := a.Continue(context.Background())
	if err != nil {
		t.Fatalf("continue failed: %v", err)
	}
	if len(recs) != 1 {
		t.Fatalf("expected 1 recommendation, got %d", len(recs))
	}
	if a.HasPendingAnalysis() {
		t.Fatalf("expected no pending analysis after successful continue")
	}
}

func TestAnalyzerCanContinueAfterTimeoutError(t *testing.T) {
	client := &sequenceClient{
		responses: []LLMResponse{
			{},
			{
				Recommendations: []models.Recommendation{{
					Path:        "D:/temp",
					Size:        100,
					Category:    models.CategorySafe,
					Reason:      "ok",
					CleanMethod: models.MethodRecycle,
					Risk:        "low",
				}},
			},
		},
		errs: []error{errors.New("llm request timeout after 2m0s: context deadline exceeded"), nil},
	}

	a := NewAnalyzer(Options{Client: client})
	_, err := a.Analyze(context.Background(), models.DirNode{Path: "D:/", Size: 1})
	if err == nil {
		t.Fatalf("expected timeout error")
	}
	if !a.HasPendingAnalysis() {
		t.Fatalf("expected pending analysis after timeout error")
	}

	recs, err := a.Continue(context.Background())
	if err != nil {
		t.Fatalf("continue failed: %v", err)
	}
	if len(recs) != 1 {
		t.Fatalf("expected 1 recommendation, got %d", len(recs))
	}
	if a.HasPendingAnalysis() {
		t.Fatalf("expected no pending analysis after successful continue")
	}
}

func TestAnalyzerKeepsSessionWhenContinueGetsTimeoutAgain(t *testing.T) {
	client := &sequenceClient{
		responses: []LLMResponse{
			{},
			{},
			{
				Recommendations: []models.Recommendation{{
					Path:        "D:/temp",
					Size:        100,
					Category:    models.CategorySafe,
					Reason:      "ok",
					CleanMethod: models.MethodRecycle,
					Risk:        "low",
				}},
			},
		},
		errs: []error{errors.New("429 Too Many Requests"), errors.New("context deadline exceeded"), nil},
	}

	a := NewAnalyzer(Options{Client: client})
	_, err := a.Analyze(context.Background(), models.DirNode{Path: "D:/", Size: 1})
	if !errors.Is(err, ErrRateLimitPaused) {
		t.Fatalf("expected rate limit pause error, got: %v", err)
	}

	_, err = a.Continue(context.Background())
	if err == nil {
		t.Fatalf("expected timeout error on continue")
	}
	if !a.HasPendingAnalysis() {
		t.Fatalf("expected pending analysis after timeout on continue")
	}

	recs, err := a.Continue(context.Background())
	if err != nil {
		t.Fatalf("second continue failed: %v", err)
	}
	if len(recs) != 1 {
		t.Fatalf("expected 1 recommendation, got %d", len(recs))
	}
}

func TestAnalyzerContinueCarriesInterruptedStreamingOutputIntoNextTurn(t *testing.T) {
	client := &interruptedStreamClient{}
	a := NewAnalyzer(Options{Client: client})

	_, err := a.Analyze(context.Background(), models.DirNode{Path: "D:/", Size: 1})
	if err == nil {
		t.Fatalf("expected interrupted streaming error")
	}
	if !a.HasPendingAnalysis() {
		t.Fatalf("expected pending analysis after interrupted stream")
	}

	recs, err := a.Continue(context.Background())
	if err != nil {
		t.Fatalf("continue failed: %v", err)
	}
	if len(recs) != 1 {
		t.Fatalf("expected 1 recommendation, got %d", len(recs))
	}
	if len(client.capturedReq) != 2 {
		t.Fatalf("expected 2 requests, got %d", len(client.capturedReq))
	}

	resumePrompt := client.capturedReq[1].User
	if !strings.Contains(resumePrompt, "流式输出在中断前已经收到部分内容") {
		t.Fatalf("expected resume prompt to mention interrupted stream, got: %s", resumePrompt)
	}
	if !strings.Contains(resumePrompt, "已发现 Downloads 下有大文件") {
		t.Fatalf("expected resume prompt to include partial assistant output, got: %s", resumePrompt)
	}
	if strings.Contains(resumePrompt, "先检查下载目录") {
		t.Fatalf("expected resume prompt to exclude partial reasoning, got: %s", resumePrompt)
	}
}

func TestAnalyzerEmitsToolCallProgress(t *testing.T) {
	client := &sequenceClient{
		responses: []LLMResponse{
			{
				Reasoning: "先检查目录内容，再给建议",
				ToolCalls: []ToolCall{{
					Name:      ToolCheckDirContent,
					Arguments: []byte(`{"path":"D:/payload"}`),
				}},
			},
			{
				Recommendations: []models.Recommendation{{
					Path:        "D:/payload",
					Size:        200,
					Category:    models.CategoryConfirm,
					Reason:      "large",
					CleanMethod: models.MethodRedirect,
					Risk:        "medium",
				}},
			},
		},
		errs: []error{nil, nil},
	}

	events := make([]AnalysisProgressEvent, 0, 8)
	a := NewAnalyzer(Options{
		Client: client,
		CheckDirContent: func(path string) (models.FileTypeDistribution, error) {
			return models.FileTypeDistribution{Path: path}, nil
		},
		OnProgress: func(event AnalysisProgressEvent) {
			events = append(events, event)
		},
	})

	_, err := a.Analyze(context.Background(), models.DirNode{Path: "D:/", Size: 1})
	if err != nil {
		t.Fatalf("analyze failed: %v", err)
	}

	found := false
	foundToolResult := false
	foundReasoning := false
	for _, event := range events {
		if event.Type == "reasoning" && strings.Contains(event.Reason, "先检查目录内容") {
			foundReasoning = true
		}
		if event.Type == "tool_call" && event.Tool == ToolCheckDirContent && event.Path == "D:/payload" && strings.Contains(event.Input, "D:/payload") {
			found = true
		}
		if event.Type == "tool_result" && event.Tool == ToolCheckDirContent && event.Path == "D:/payload" && strings.Contains(event.Output, "D:/payload") {
			foundToolResult = true
		}
	}
	if !found {
		t.Fatalf("expected tool_call event with path, got %+v", events)
	}
	if !foundToolResult {
		t.Fatalf("expected tool_result event with output, got %+v", events)
	}
	if !foundReasoning {
		t.Fatalf("expected reasoning event, got %+v", events)
	}
}

func TestParseSubmitRecommendationsSupportsWrappedJSON(t *testing.T) {
	raw := []byte(`{"recommendations":[{"path":"D:/temp","size":123,"category":"safe","reason":"ok","clean_method":"recycle","command":"","risk":"low"}]}`)
	recs, err := parseSubmitRecommendations(raw)
	if err != nil {
		t.Fatalf("parse submit recommendations failed: %v", err)
	}
	if len(recs) != 1 {
		t.Fatalf("expected 1 recommendation, got %d", len(recs))
	}
}

func TestParseSubmitRecommendationsSupportsQuotedPayload(t *testing.T) {
	raw := []byte(`"{\"recommendations\":[{\"path\":\"D:/temp\",\"size\":123,\"category\":\"safe\",\"reason\":\"ok\",\"clean_method\":\"recycle\",\"command\":\"\",\"risk\":\"low\"}]}"`)
	recs, err := parseSubmitRecommendations(raw)
	if err != nil {
		t.Fatalf("parse quoted submit recommendations failed: %v", err)
	}
	if len(recs) != 1 {
		t.Fatalf("expected 1 recommendation, got %d", len(recs))
	}
}

func TestAnalyzerResolvesSizeFromClientScan(t *testing.T) {
	client := &sequenceClient{
		responses: []LLMResponse{
			{
				ToolCalls: []ToolCall{{
					Name:      ToolSubmitRecommendations,
					Arguments: []byte(`{"recommendations":[{"path":"D:/payload","category":"confirm","reason":"x","clean_method":"recycle","command":"","risk":"low"}]}`),
				}},
			},
		},
		errs: []error{nil},
	}

	a := NewAnalyzer(Options{
		Client: client,
		ScanDeeper: func(path string, depth int) (models.DirNode, error) {
			return models.DirNode{Path: path, Size: 123456789}, nil
		},
	})

	recs, err := a.Analyze(context.Background(), models.DirNode{Path: "D:/", Size: 1})
	if err != nil {
		t.Fatalf("analyze failed: %v", err)
	}
	if len(recs) != 1 {
		t.Fatalf("expected 1 recommendation, got %d", len(recs))
	}
	if recs[0].Size != 123456789 {
		t.Fatalf("expected size from client scan, got %d", recs[0].Size)
	}
}

func TestAnalyzerContinuesWhenToolExecutionFails(t *testing.T) {
	client := &sequenceClient{
		responses: []LLMResponse{
			{
				ToolCalls: []ToolCall{{
					Name:      ToolScanDeeper,
					Arguments: []byte(`{"path":"D:/missing","depth":2}`),
				}},
			},
			{
				Recommendations: []models.Recommendation{{
					Path:        "D:/temp/cache",
					Size:        100,
					Category:    models.CategorySafe,
					Reason:      "cache",
					CleanMethod: models.MethodRecycle,
					Risk:        "low",
				}},
			},
		},
		errs: []error{nil, nil},
	}

	events := make([]AnalysisProgressEvent, 0, 8)
	a := NewAnalyzer(Options{
		Client: client,
		ScanDeeper: func(path string, depth int) (models.DirNode, error) {
			return models.DirNode{}, fmt.Errorf("path not found: %s", path)
		},
		OnProgress: func(event AnalysisProgressEvent) {
			events = append(events, event)
		},
	})

	recs, err := a.Analyze(context.Background(), models.DirNode{Path: "D:/", Size: 1})
	if err != nil {
		t.Fatalf("analyze should continue after tool error, got: %v", err)
	}
	if len(recs) != 1 {
		t.Fatalf("expected 1 recommendation, got %d", len(recs))
	}

	foundToolErr := false
	for _, event := range events {
		if event.Type == "tool_result" && event.Tool == ToolScanDeeper && strings.Contains(strings.ToLower(event.Output), "path not found") {
			foundToolErr = true
			break
		}
	}
	if !foundToolErr {
		t.Fatalf("expected tool_result error event, got %+v", events)
	}
}
