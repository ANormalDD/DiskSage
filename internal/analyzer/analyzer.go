package analyzer

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"disksage/internal/models"
	"disksage/internal/scanner"
)

type LLMRequest struct {
	System     string
	User       string
	Tools      []ToolDefinition
	ToolChoice string
	Config     models.LLMConfig
}

type LLMResponse struct {
	Content         string
	Reasoning       string
	RawResponse     string
	ToolCalls       []ToolCall
	Recommendations []models.Recommendation
	Usage           models.TokenUsage
}

type LLMClient interface {
	Complete(ctx context.Context, req LLMRequest) (LLMResponse, error)
}

type TreeAwareClient interface {
	AnalyzeTree(ctx context.Context, root models.DirNode) ([]models.Recommendation, error)
}

type Options struct {
	Client          LLMClient
	Config          models.LLMConfig
	ScanDeeper      func(path string, depth int) (models.DirNode, error)
	CheckDirContent func(path string) (models.FileTypeDistribution, error)
	OnProgress      func(event AnalysisProgressEvent)
}

var ErrRateLimitPaused = errors.New("analysis paused by rate limit")

type AnalysisProgressEvent struct {
	Type    string `json:"type"`
	Turn    int    `json:"turn"`
	Tool    string `json:"tool"`
	Path    string `json:"path"`
	Content string `json:"content"`
	Reason  string `json:"reason"`
	Input   string `json:"input"`
	Output  string `json:"output"`
	At      string `json:"at"`
}

type analysisSession struct {
	Root              models.DirNode
	System            string
	User              string
	CtxNotes          []string
	RawOutputs        []string
	ToolChoice        string
	NonCompliantTurns int
	AutoProbeDone     bool
	NextTurn          int
	MaxTurns          int
}

type Analyzer struct {
	mu              sync.RWMutex
	client          LLMClient
	cfg             models.LLMConfig
	scanDeeper      func(path string, depth int) (models.DirNode, error)
	checkDirContent func(path string) (models.FileTypeDistribution, error)

	statsMu    sync.Mutex
	tokenStats models.TokenStats

	debugMu   sync.Mutex
	debugInfo models.LLMDebugInfo

	progressMu sync.RWMutex
	onProgress func(event AnalysisProgressEvent)

	sessionMu sync.Mutex
	session   *analysisSession
}

func NewAnalyzer(opts Options) *Analyzer {
	return &Analyzer{
		client:          opts.Client,
		cfg:             opts.Config,
		scanDeeper:      opts.ScanDeeper,
		checkDirContent: opts.CheckDirContent,
		onProgress:      opts.OnProgress,
	}
}

func (a *Analyzer) UpdateConfig(cfg models.LLMConfig) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.cfg = cfg
}

func (a *Analyzer) SetClient(client LLMClient) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.client = client
}

func (a *Analyzer) SetProgressCallback(cb func(event AnalysisProgressEvent)) {
	a.progressMu.Lock()
	defer a.progressMu.Unlock()
	a.onProgress = cb
}

func (a *Analyzer) emitProgress(event AnalysisProgressEvent) {
	event.At = time.Now().Format(time.RFC3339Nano)
	a.progressMu.RLock()
	cb := a.onProgress
	a.progressMu.RUnlock()
	if cb != nil {
		cb(event)
	}
}

func (a *Analyzer) HasPendingAnalysis() bool {
	a.sessionMu.Lock()
	defer a.sessionMu.Unlock()
	return a.session != nil
}

func (a *Analyzer) Continue(ctx context.Context) ([]models.Recommendation, error) {
	a.mu.RLock()
	client := a.client
	cfg := a.cfg
	scanDeeper := a.scanDeeper
	checkDirContent := a.checkDirContent
	a.mu.RUnlock()

	a.sessionMu.Lock()
	session := a.session
	a.sessionMu.Unlock()
	if session == nil {
		return nil, fmt.Errorf("no pending analysis session")
	}

	a.emitProgress(AnalysisProgressEvent{Type: "resume", Turn: session.NextTurn, Content: "继续迭代分析"})
	recs, err := a.runSession(ctx, session, client, cfg, scanDeeper, checkDirContent)
	if err == nil || !errors.Is(err, ErrRateLimitPaused) {
		a.sessionMu.Lock()
		if a.session == session {
			a.session = nil
		}
		a.sessionMu.Unlock()
	}
	return recs, err
}

func (a *Analyzer) GetTokenStats() models.TokenStats {
	a.statsMu.Lock()
	defer a.statsMu.Unlock()
	return a.tokenStats
}

func (a *Analyzer) recordUsage(usage models.TokenUsage) {
	normalized := normalizeUsage(usage)
	a.statsMu.Lock()
	defer a.statsMu.Unlock()
	a.tokenStats.Last = normalized
	a.tokenStats.Total.InputTokens += normalized.InputTokens
	a.tokenStats.Total.OutputTokens += normalized.OutputTokens
	a.tokenStats.Total.TotalTokens += normalized.TotalTokens
	a.tokenStats.RequestCount++
}

func (a *Analyzer) GetDebugInfo() models.LLMDebugInfo {
	a.debugMu.Lock()
	defer a.debugMu.Unlock()
	return a.debugInfo
}

func (a *Analyzer) recordDebug(rawOutput, lastError, source string) {
	a.debugMu.Lock()
	defer a.debugMu.Unlock()
	a.debugInfo = models.LLMDebugInfo{
		RawOutput: rawOutput,
		LastError: lastError,
		Source:    source,
		UpdatedAt: time.Now(),
	}
}

func (a *Analyzer) Analyze(ctx context.Context, root models.DirNode) ([]models.Recommendation, error) {
	a.mu.RLock()
	client := a.client
	cfg := a.cfg
	scanDeeper := a.scanDeeper
	checkDirContent := a.checkDirContent
	a.mu.RUnlock()

	compressed := scanner.RenderCompressedTree(root, scanner.RenderConfig{
		TopNPerLevel: 20,
		MinChildSize: 50 * 1024 * 1024,
	})
	system, user := BuildPrompt(compressed)
	analysisUsage := models.TokenUsage{}
	maxTurns := cfg.MaxTurns
	if maxTurns == 0 {
		maxTurns = 6
	}

	if client == nil {
		recs := heuristicRecommendations(root)
		analysisUsage = estimateUsageFromTexts(system+"\n"+user, recommendationsAsText(recs))
		a.recordUsage(analysisUsage)
		a.recordDebug("", "", "heuristic")
		return recs, nil
	}
	if treeClient, ok := client.(TreeAwareClient); ok {
		recs, err := treeClient.AnalyzeTree(ctx, root)
		if err != nil {
			a.recordDebug("", err.Error(), "heuristic")
			return nil, err
		}
		analysisUsage = estimateUsageFromTexts(system+"\n"+user, recommendationsAsText(recs))
		a.recordUsage(analysisUsage)
		a.recordDebug("", "", "heuristic")
		return recs, nil
	}
	session := &analysisSession{
		Root:       root,
		System:     system,
		User:       user,
		CtxNotes:   make([]string, 0, 8),
		RawOutputs: make([]string, 0, 8),
		ToolChoice: "auto",
		NextTurn:   1,
		MaxTurns:   maxTurns,
	}
	a.sessionMu.Lock()
	a.session = session
	a.sessionMu.Unlock()

	recs, err := a.runSession(ctx, session, client, cfg, scanDeeper, checkDirContent)
	if err == nil || !errors.Is(err, ErrRateLimitPaused) {
		a.sessionMu.Lock()
		if a.session == session {
			a.session = nil
		}
		a.sessionMu.Unlock()
	}
	return recs, err
}

func (a *Analyzer) runSession(ctx context.Context, session *analysisSession, client LLMClient, cfg models.LLMConfig, scanDeeper func(path string, depth int) (models.DirNode, error), checkDirContent func(path string) (models.FileTypeDistribution, error)) ([]models.Recommendation, error) {
	analysisUsage := models.TokenUsage{}
	for session.MaxTurns < 0 || session.NextTurn <= session.MaxTurns {
		turn := session.NextTurn
		if err := ctx.Err(); err != nil {
			a.recordUsage(analysisUsage)
			a.recordDebug(strings.Join(session.RawOutputs, "\n\n"), err.Error(), "llm")
			return nil, fmt.Errorf("analysis interrupted on turn %d: %w", turn, err)
		}

		turnUser := buildTurnUserPrompt(session.User, session.CtxNotes)
		resp, err := client.Complete(ctx, LLMRequest{
			System:     session.System,
			User:       turnUser,
			Tools:      DefaultToolDefinitions(),
			ToolChoice: session.ToolChoice,
			Config:     cfg,
		})
		analysisUsage = addUsage(analysisUsage, resp.Usage)
		if err != nil {
			if session.ToolChoice == "required" && strings.Contains(strings.ToLower(err.Error()), "tool_choice") {
				session.ToolChoice = "auto"
				session.CtxNotes = append(session.CtxNotes, "当前模型端点可能不支持 tool_choice=required。请改为严格按工具调用或直接输出 JSON 数组。")
				session.NextTurn++
				continue
			}
			analysisUsage = addUsage(analysisUsage, estimateUsageFromTexts(session.System+"\n"+turnUser, ""))
			a.recordUsage(analysisUsage)
			a.recordDebug(strings.Join(session.RawOutputs, "\n\n"), err.Error(), "llm")
			if isRateLimitError(err) {
				a.emitProgress(AnalysisProgressEvent{Type: "paused_rate_limit", Turn: turn, Content: err.Error()})
				return nil, fmt.Errorf("%w: %v", ErrRateLimitPaused, err)
			}
			return nil, fmt.Errorf("llm analyze failed on turn %d: %w", turn, err)
		}

		rawResp := strings.TrimSpace(resp.RawResponse)
		content := strings.TrimSpace(resp.Content)
		reasoning := strings.TrimSpace(resp.Reasoning)
		if rawResp != "" {
			session.RawOutputs = append(session.RawOutputs, fmt.Sprintf("[turn-%d api-raw]\n%s", turn, rawResp))
		} else if content != "" {
			session.RawOutputs = append(session.RawOutputs, fmt.Sprintf("[turn-%d parsed-content]\n%s", turn, content))
		}
		if reasoning != "" {
			a.emitProgress(AnalysisProgressEvent{Type: "reasoning", Turn: turn, Content: "模型推理", Reason: truncateProgressText(reasoning, 12000)})
		}
		if content != "" {
			a.emitProgress(AnalysisProgressEvent{Type: "assistant_text", Turn: turn, Content: truncateProgressText(content, 12000)})
		}

		if len(resp.Recommendations) > 0 {
			recs := sanitizeRecommendations(resp.Recommendations)
			if len(recs) > 0 {
				recs = resolveRecommendationSizes(recs, scanDeeper)
				if normalizeUsage(analysisUsage).TotalTokens == 0 {
					analysisUsage = estimateUsageFromTexts(session.System+"\n"+turnUser, recommendationsAsText(recs))
				}
				a.recordUsage(analysisUsage)
				a.recordDebug(strings.Join(session.RawOutputs, "\n\n"), "", "llm")
				a.emitProgress(AnalysisProgressEvent{Type: "completed", Turn: turn, Content: "分析完成"})
				return recs, nil
			}
		}

		if len(resp.ToolCalls) > 0 {
			session.NonCompliantTurns = 0
			session.ToolChoice = "auto"
			for _, tc := range resp.ToolCalls {
				a.emitProgress(AnalysisProgressEvent{Type: "tool_call", Turn: turn, Tool: tc.Name, Path: extractToolPath(tc), Input: formatJSONForProgress(tc.Arguments)})
			}
			toolResults, submitRecs, toolErr := executeToolCalls(resp.ToolCalls, scanDeeper, checkDirContent, func(name, path, input, output string) {
				a.emitProgress(AnalysisProgressEvent{Type: "tool_result", Turn: turn, Tool: name, Path: path, Input: truncateProgressText(input, 12000), Output: truncateProgressText(output, 12000)})
			})
			if toolErr != nil {
				if strings.Contains(strings.ToLower(toolErr.Error()), "submit_recommendations arguments invalid") {
					if parsed, parseErr := ParseRecommendations(content); parseErr == nil {
						parsed = resolveRecommendationSizes(parsed, scanDeeper)
						if normalizeUsage(analysisUsage).TotalTokens == 0 {
							analysisUsage = estimateUsageFromTexts(session.System+"\n"+turnUser, content)
						}
						a.recordUsage(analysisUsage)
						a.recordDebug(strings.Join(session.RawOutputs, "\n\n"), "", "llm")
						a.emitProgress(AnalysisProgressEvent{Type: "completed", Turn: turn, Content: "分析完成（回退为直接解析）"})
						return parsed, nil
					}
					session.ToolChoice = "required"
					session.CtxNotes = append(session.CtxNotes,
						"submit_recommendations 参数格式无效。下一轮必须仅调用 submit_recommendations，arguments 必须是严格 JSON 对象：{\"recommendations\":[...] }，不要在参数中混入解释文字。",
					)
					session.NextTurn++
					continue
				}
				a.recordUsage(analysisUsage)
				a.recordDebug(strings.Join(session.RawOutputs, "\n\n"), toolErr.Error(), "llm")
				return nil, toolErr
			}
			if len(submitRecs) > 0 {
				submitRecs = resolveRecommendationSizes(submitRecs, scanDeeper)
				if normalizeUsage(analysisUsage).TotalTokens == 0 {
					analysisUsage = estimateUsageFromTexts(session.System+"\n"+turnUser, recommendationsAsText(submitRecs))
				}
				a.recordUsage(analysisUsage)
				a.recordDebug(strings.Join(session.RawOutputs, "\n\n"), "", "llm")
				a.emitProgress(AnalysisProgressEvent{Type: "completed", Turn: turn, Content: "分析完成"})
				return submitRecs, nil
			}
			if len(toolResults) > 0 {
				session.CtxNotes = append(session.CtxNotes, "工具调用结果：\n"+strings.Join(toolResults, "\n\n"))
				session.NextTurn++
				continue
			}
			session.NonCompliantTurns++
			session.ToolChoice = "required"
			session.CtxNotes = append(session.CtxNotes, "你返回了工具调用，但参数无效。请严格按工具 schema 重新调用，或直接输出有效 JSON 数组。")
			session.NextTurn++
			continue
		}

		if content != "" {
			if parsed, parseErr := ParseRecommendations(content); parseErr == nil {
				parsed = resolveRecommendationSizes(parsed, scanDeeper)
				if normalizeUsage(analysisUsage).TotalTokens == 0 {
					analysisUsage = estimateUsageFromTexts(session.System+"\n"+turnUser, content)
				}
				a.recordUsage(analysisUsage)
				a.recordDebug(strings.Join(session.RawOutputs, "\n\n"), "", "llm")
				a.emitProgress(AnalysisProgressEvent{Type: "completed", Turn: turn, Content: "分析完成"})
				return parsed, nil
			}
			session.NonCompliantTurns++
			session.ToolChoice = "required"
			if !session.AutoProbeDone && session.NonCompliantTurns >= 2 {
				session.AutoProbeDone = true
				if probes := autoProbeLargeDirs(session.Root, scanDeeper); len(probes) > 0 {
					session.CtxNotes = append(session.CtxNotes, "系统自动补充深扫结果：\n"+strings.Join(probes, "\n\n"))
				}
			}
			session.CtxNotes = append(session.CtxNotes,
				"上轮输出不是有效 JSON 数组，也未提供可执行 tool_call。你必须二选一：\n"+
					"1) 调用 scan_deeper/check_dir_content 然后调用 submit_recommendations；\n"+
					"2) 直接输出 JSON 数组，且不要任何解释文字。\n"+
					"上轮原文：\n"+content,
			)
			session.NextTurn++
			continue
		}

		session.NonCompliantTurns++
		session.ToolChoice = "required"
		if !session.AutoProbeDone && session.NonCompliantTurns >= 2 {
			session.AutoProbeDone = true
			if probes := autoProbeLargeDirs(session.Root, scanDeeper); len(probes) > 0 {
				session.CtxNotes = append(session.CtxNotes, "系统自动补充深扫结果：\n"+strings.Join(probes, "\n\n"))
			}
		}
		session.CtxNotes = append(session.CtxNotes, "你尚未给出可解析输出。请直接返回 JSON 数组或 tool_call。")
		session.NextTurn++
	}

	if normalizeUsage(analysisUsage).TotalTokens == 0 {
		analysisUsage = estimateUsageFromTexts(session.System+"\n"+session.User, strings.Join(session.RawOutputs, "\n"))
	}
	a.recordUsage(analysisUsage)
	a.recordDebug(strings.Join(session.RawOutputs, "\n\n"), "model did not finish in structured format", "llm")
	if session.MaxTurns < 0 {
		return nil, fmt.Errorf("llm did not return structured output before context cancellation")
	}
	return nil, fmt.Errorf("llm did not return structured output after %d turns", session.MaxTurns)
}

func isRateLimitError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "rate limit") || strings.Contains(msg, "too many requests") || strings.Contains(msg, " 429") || strings.Contains(msg, "\"429\"") || strings.Contains(msg, "status: 429")
}

func extractToolPath(tc ToolCall) string {
	var args map[string]any
	if err := json.Unmarshal(tc.Arguments, &args); err != nil {
		return ""
	}
	path := strings.TrimSpace(fmt.Sprintf("%v", args["path"]))
	if path == "<nil>" {
		return ""
	}
	return path
}

func formatJSONForProgress(raw json.RawMessage) string {
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" {
		return "{}"
	}

	var payload any
	if err := json.Unmarshal(raw, &payload); err == nil {
		blob, marshalErr := json.MarshalIndent(payload, "", "  ")
		if marshalErr == nil {
			return truncateProgressText(string(blob), 12000)
		}
	}

	return truncateProgressText(trimmed, 12000)
}

func truncateProgressText(text string, maxLen int) string {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" || maxLen <= 0 {
		return trimmed
	}
	if len(trimmed) <= maxLen {
		return trimmed
	}
	return trimmed[:maxLen] + "\n...(截断)"
}

func buildTurnUserPrompt(base string, notes []string) string {
	if len(notes) == 0 {
		return base
	}
	return base + "\n\n会话上下文（按时间顺序）：\n" + strings.Join(notes, "\n\n")
}

func autoProbeLargeDirs(root models.DirNode, scanDeeper func(path string, depth int) (models.DirNode, error)) []string {
	if scanDeeper == nil {
		return nil
	}

	candidates := make([]models.DirNode, 0, len(root.Children))
	for _, child := range root.Children {
		if child.IsFile {
			continue
		}
		if child.Size < 100*1024*1024 {
			continue
		}
		candidates = append(candidates, child)
	}
	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].Size > candidates[j].Size
	})
	if len(candidates) > 3 {
		candidates = candidates[:3]
	}

	results := make([]string, 0, len(candidates))
	for _, c := range candidates {
		node, err := scanDeeper(c.Path, 3)
		if err != nil {
			continue
		}
		results = append(results, "auto scan_deeper result:\n"+scanner.RenderCompressedTree(node, scanner.RenderConfig{TopNPerLevel: 15, MinChildSize: 20 * 1024 * 1024}))
	}
	return results
}

func executeToolCalls(toolCalls []ToolCall, scanDeeper func(path string, depth int) (models.DirNode, error), checkDirContent func(path string) (models.FileTypeDistribution, error), onExecuted func(name, path, input, output string)) ([]string, []models.Recommendation, error) {
	results := make([]string, 0, len(toolCalls))
	for _, tc := range toolCalls {
		switch tc.Name {
		case ToolSubmitRecommendations:
			recs, err := parseSubmitRecommendations(tc.Arguments)
			if err != nil {
				return nil, nil, fmt.Errorf("submit_recommendations arguments invalid: %w", err)
			}
			if len(recs) > 0 {
				if onExecuted != nil {
					onExecuted(tc.Name, "", formatJSONForProgress(tc.Arguments), fmt.Sprintf("submit_recommendations accepted %d item(s)", len(recs)))
				}
				return nil, recs, nil
			}
		case ToolScanDeeper:
			if scanDeeper == nil {
				continue
			}
			var args struct {
				Path  string `json:"path"`
				Depth int    `json:"depth"`
			}
			if err := json.Unmarshal(tc.Arguments, &args); err != nil {
				return nil, nil, fmt.Errorf("scan_deeper arguments invalid: %w", err)
			}
			if args.Depth <= 0 {
				args.Depth = 3
			}
			child, err := scanDeeper(args.Path, args.Depth)
			if err != nil {
				return nil, nil, fmt.Errorf("scan_deeper failed: %w", err)
			}
			rendered := scanner.RenderCompressedTree(child, scanner.RenderConfig{TopNPerLevel: 20, MinChildSize: 10 * 1024 * 1024})
			if onExecuted != nil {
				onExecuted(tc.Name, args.Path, formatJSONForProgress(tc.Arguments), rendered)
			}
			results = append(results, "scan_deeper result:\n"+rendered)
		case ToolCheckDirContent:
			if checkDirContent == nil {
				continue
			}
			var args struct {
				Path string `json:"path"`
			}
			if err := json.Unmarshal(tc.Arguments, &args); err != nil {
				return nil, nil, fmt.Errorf("check_dir_content arguments invalid: %w", err)
			}
			dist, err := checkDirContent(args.Path)
			if err != nil {
				return nil, nil, fmt.Errorf("check_dir_content failed: %w", err)
			}
			blob, _ := json.MarshalIndent(dist, "", "  ")
			if onExecuted != nil {
				onExecuted(tc.Name, args.Path, formatJSONForProgress(tc.Arguments), string(blob))
			}
			results = append(results, "check_dir_content result:\n"+string(blob))
		}
	}

	return results, nil, nil
}

func parseSubmitRecommendations(arguments json.RawMessage) ([]models.Recommendation, error) {
	var wrapped struct {
		Recommendations []models.Recommendation `json:"recommendations"`
	}
	if err := json.Unmarshal(arguments, &wrapped); err == nil {
		recs := sanitizeRecommendations(wrapped.Recommendations)
		if len(recs) > 0 {
			return recs, nil
		}
	}

	text := strings.TrimSpace(string(arguments))
	if parsed, err := ParseRecommendations(text); err == nil && len(parsed) > 0 {
		return parsed, nil
	}

	var quoted string
	if err := json.Unmarshal(arguments, &quoted); err == nil {
		if parsed, parseErr := ParseRecommendations(quoted); parseErr == nil && len(parsed) > 0 {
			return parsed, nil
		}
	}

	return nil, fmt.Errorf("unable to parse recommendations from submit arguments")
}

func resolveRecommendationSizes(recs []models.Recommendation, scanDeeper func(path string, depth int) (models.DirNode, error)) []models.Recommendation {
	if len(recs) == 0 {
		return recs
	}

	resolved := make([]models.Recommendation, 0, len(recs))
	for _, rec := range recs {
		if strings.TrimSpace(rec.Path) == "" {
			continue
		}
		if scanDeeper != nil {
			node, err := scanDeeper(rec.Path, 1)
			if err == nil && node.Size > 0 {
				rec.Size = node.Size
			}
		}
		if rec.Size < 0 {
			rec.Size = 0
		}
		resolved = append(resolved, rec)
	}

	return resolved
}

func addUsage(a1, a2 models.TokenUsage) models.TokenUsage {
	return models.TokenUsage{
		InputTokens:  a1.InputTokens + a2.InputTokens,
		OutputTokens: a1.OutputTokens + a2.OutputTokens,
		TotalTokens:  a1.TotalTokens + a2.TotalTokens,
	}
}

func normalizeUsage(u models.TokenUsage) models.TokenUsage {
	if u.InputTokens < 0 {
		u.InputTokens = 0
	}
	if u.OutputTokens < 0 {
		u.OutputTokens = 0
	}
	if u.TotalTokens <= 0 {
		u.TotalTokens = u.InputTokens + u.OutputTokens
	}
	if u.TotalTokens < u.InputTokens+u.OutputTokens {
		u.TotalTokens = u.InputTokens + u.OutputTokens
	}
	return u
}

func estimateUsageFromTexts(inputText, outputText string) models.TokenUsage {
	in := estimateTokens(inputText)
	out := estimateTokens(outputText)
	return models.TokenUsage{
		InputTokens:  in,
		OutputTokens: out,
		TotalTokens:  in + out,
	}
}

func estimateTokens(text string) int {
	runes := len([]rune(text))
	if runes <= 0 {
		return 0
	}
	return int(math.Ceil(float64(runes) / 4.0))
}

func recommendationsAsText(recs []models.Recommendation) string {
	blob, _ := json.Marshal(recs)
	return string(blob)
}

type HeuristicClient struct{}

func NewHeuristicClient() *HeuristicClient {
	return &HeuristicClient{}
}

func (h *HeuristicClient) Complete(ctx context.Context, req LLMRequest) (LLMResponse, error) {
	_ = ctx
	_ = req
	return LLMResponse{}, fmt.Errorf("heuristic client only supports AnalyzeTree")
}

func (h *HeuristicClient) AnalyzeTree(ctx context.Context, root models.DirNode) ([]models.Recommendation, error) {
	_ = ctx
	return heuristicRecommendations(root), nil
}

func heuristicRecommendations(root models.DirNode) []models.Recommendation {
	result := make([]models.Recommendation, 0, 32)
	seen := map[string]struct{}{}

	var walk func(n models.DirNode)
	walk = func(n models.DirNode) {
		if n.Path != "" {
			if rec, ok := classifyNode(n); ok {
				if _, exists := seen[rec.Path]; !exists {
					seen[rec.Path] = struct{}{}
					result = append(result, rec)
				}
			}
		}
		for _, c := range n.Children {
			walk(c)
		}
	}
	walk(root)

	sort.Slice(result, func(i, j int) bool {
		if result[i].Category == result[j].Category {
			return result[i].Size > result[j].Size
		}
		order := map[models.CleanCategory]int{
			models.CategorySafe:    0,
			models.CategoryConfirm: 1,
			models.CategoryManual:  2,
			models.CategoryReview:  3,
		}
		return order[result[i].Category] < order[result[j].Category]
	})
	return result
}

func classifyNode(n models.DirNode) (models.Recommendation, bool) {
	if n.Size < 50*1024*1024 {
		return models.Recommendation{}, false
	}
	pathLower := strings.ToLower(filepath.ToSlash(n.Path))
	nameLower := strings.ToLower(n.Name)

	if strings.Contains(pathLower, "/windows") || strings.Contains(pathLower, "/program files") {
		return models.Recommendation{}, false
	}

	if strings.Contains(pathLower, "/temp") || strings.Contains(pathLower, "/cache") || strings.Contains(pathLower, "/logs") || strings.Contains(pathLower, ".gradle/caches") || strings.Contains(pathLower, "go/pkg/mod") || strings.Contains(pathLower, ".cargo/registry") {
		return models.Recommendation{
			Path:        n.Path,
			Size:        n.Size,
			Category:    models.CategorySafe,
			Reason:      "缓存/临时文件可安全清理",
			CleanMethod: models.MethodRecycle,
			Risk:        "低风险，必要时可重新生成",
		}, true
	}

	if strings.Contains(pathLower, "/downloads") || strings.HasSuffix(nameLower, ".iso") || strings.HasSuffix(nameLower, ".zip") {
		return models.Recommendation{
			Path:        n.Path,
			Size:        n.Size,
			Category:    models.CategoryConfirm,
			Reason:      "下载目录或归档文件，建议先确认是否仍需要",
			CleanMethod: models.MethodRedirect,
			Risk:        "删除后可能无法恢复原始安装包/归档",
		}, true
	}

	if strings.Contains(pathLower, "node_modules") {
		inactive := time.Since(n.ModTime) > 120*24*time.Hour
		if inactive {
			return models.Recommendation{
				Path:        n.Path,
				Size:        n.Size,
				Category:    models.CategoryManual,
				Reason:      "疑似非活跃项目依赖目录",
				CleanMethod: models.MethodCommand,
				Command:     "npm cache clean --force",
				Risk:        "下次构建会重新下载依赖",
			}, true
		}
	}

	if strings.Contains(pathLower, "docker") {
		return models.Recommendation{
			Path:        n.Path,
			Size:        n.Size,
			Category:    models.CategoryManual,
			Reason:      "Docker 相关缓存可能占用大量空间",
			CleanMethod: models.MethodCommand,
			Command:     "docker system prune -a",
			Risk:        "会移除未使用镜像/容器",
		}, true
	}

	if n.Size >= 2*1024*1024*1024 && time.Since(n.ModTime) > 180*24*time.Hour {
		return models.Recommendation{
			Path:        n.Path,
			Size:        n.Size,
			Category:    models.CategoryReview,
			Reason:      "大目录长期未变更，建议人工检查",
			CleanMethod: models.MethodRedirect,
			Risk:        "用途不明确，需人工判断",
		}, true
	}

	return models.Recommendation{}, false
}
