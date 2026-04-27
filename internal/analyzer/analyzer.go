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

// ConversationMessage represents a single entry in the OpenAI-compatible multi-turn message history.
type ConversationMessage struct {
	Role             string          `json:"role"`                        // "system", "user", "assistant", "tool"
	Content          string          `json:"content"`
	ReasoningContent string          `json:"reasoning_content,omitempty"` // thinking text; must be passed back verbatim for thinking-mode models
	ToolCalls        []ToolCall      `json:"tool_calls,omitempty"`        // set when role=="assistant" and model called tools
	ToolCallID       string          `json:"tool_call_id,omitempty"`      // set when role=="tool"
	Name             string          `json:"name,omitempty"`              // tool name, set when role=="tool"
	RawArgs          json.RawMessage `json:"-"`                           // raw args for assistant tool call reconstruction
}

type LLMRequest struct {
	System        string
	User          string
	Messages      []ConversationMessage // if non-empty, used instead of System+User two-message format
	Tools         []ToolDefinition
	ToolChoice    string
	Config        models.LLMConfig
	OnStreamDelta func(delta LLMStreamDelta)
}

type LLMStreamDelta struct {
	Content   string
	Reasoning string
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
	// Messages holds the full conversation history (excluding system) sent to the LLM each turn.
	// Turn 1: [user]. Turn 2+: [user, assistant, tool, tool, ...assistant, tool, ...].
	Messages          []ConversationMessage
	RawOutputs        []string
	ToolChoice        string
	NonCompliantTurns int
	AutoProbeDone     bool
	NextTurn          int
	MaxTurns          int
	AccumulatedRecs   []models.Recommendation
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
	if err == nil || !isResumableAnalysisError(err) {
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
	system, user := BuildPrompt(compressed, cfg)
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
		Root:     root,
		System:   system,
		User:     user,
		Messages: []ConversationMessage{
			{Role: "user", Content: user},
		},
		RawOutputs: make([]string, 0, 8),
		ToolChoice: "auto",
		NextTurn:   1,
		MaxTurns:   maxTurns,
	}
	a.sessionMu.Lock()
	a.session = session
	a.sessionMu.Unlock()

	recs, err := a.runSession(ctx, session, client, cfg, scanDeeper, checkDirContent)
	if err == nil || !isResumableAnalysisError(err) {
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

	// appendUserNote adds a user-role correction note to the message history.
	appendUserNote := func(note string) {
		session.Messages = append(session.Messages, ConversationMessage{Role: "user", Content: note})
	}

	for session.MaxTurns < 0 || session.NextTurn <= session.MaxTurns {
		turn := session.NextTurn
		if err := ctx.Err(); err != nil {
			a.recordUsage(analysisUsage)
			a.recordDebug(strings.Join(session.RawOutputs, "\n\n"), err.Error(), "llm")
			return nil, fmt.Errorf("analysis interrupted on turn %d: %w", turn, err)
		}

		streamedAny := false
		streamedContent := strings.Builder{}
		resp, err := client.Complete(ctx, LLMRequest{
			System:     session.System,
			Messages:   session.Messages,
			Tools:      BuildToolDefinitions(cfg),
			ToolChoice: session.ToolChoice,
			Config:     cfg,
			OnStreamDelta: func(delta LLMStreamDelta) {
				if delta.Reasoning != "" {
					streamedAny = true
					a.emitProgress(AnalysisProgressEvent{Type: "reasoning", Turn: turn, Content: "模型推理（流式）", Reason: truncateProgressText(delta.Reasoning, 12000)})
				}
				if delta.Content != "" {
					streamedAny = true
					streamedContent.WriteString(delta.Content)
					a.emitProgress(AnalysisProgressEvent{Type: "assistant_text", Turn: turn, Content: truncateProgressText(delta.Content, 12000)})
				}
			},
		})
		analysisUsage = addUsage(analysisUsage, resp.Usage)
		if err != nil {
			// If we got partial streamed content before the error, inject it as an assistant message
			// so the next turn knows what was already said.
			if partial := strings.TrimSpace(streamedContent.String()); partial != "" {
				session.Messages = append(session.Messages, ConversationMessage{Role: "assistant", Content: partial})
				note := fmt.Sprintf("上一轮（turn %d）流式输出在中断前已经收到部分内容。不要重复这些内容，请基于它们继续完成当前分析。\n\n已收到的回答片段：\n%s", turn, partial)
				appendUserNote(note)
				session.RawOutputs = append(session.RawOutputs, note)
				session.NextTurn++
			}
			if session.ToolChoice == "required" && strings.Contains(strings.ToLower(err.Error()), "tool_choice") {
				session.ToolChoice = "auto"
				appendUserNote("当前模型端点可能不支持 tool_choice=required。请改为严格按工具调用或直接输出 JSON 数组。")
				session.NextTurn++
				continue
			}
			analysisUsage = addUsage(analysisUsage, estimateUsageFromTexts(session.System, ""))
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
		if reasoning != "" && !streamedAny {
			a.emitProgress(AnalysisProgressEvent{Type: "reasoning", Turn: turn, Content: "模型推理", Reason: truncateProgressText(reasoning, 12000)})
		}
		if content != "" && !streamedAny {
			a.emitProgress(AnalysisProgressEvent{Type: "assistant_text", Turn: turn, Content: truncateProgressText(content, 12000)})
		}

		// --- PATH 1: LLM directly returned parsed recommendations (no-tools fallback) ---
		if len(resp.Recommendations) > 0 {
			recs := sanitizeRecommendations(resp.Recommendations)
			if len(recs) > 0 {
				recs = resolveRecommendationSizes(recs, scanDeeper)
				if normalizeUsage(analysisUsage).TotalTokens == 0 {
					analysisUsage = estimateUsageFromTexts(session.System, recommendationsAsText(recs))
				}
				a.recordUsage(analysisUsage)
				a.recordDebug(strings.Join(session.RawOutputs, "\n\n"), "", "llm")
				a.emitProgress(AnalysisProgressEvent{Type: "completed", Turn: turn, Content: "分析完成"})
				return recs, nil
			}
		}

		// --- PATH 2: LLM called tools ---
		if len(resp.ToolCalls) > 0 {
			session.NonCompliantTurns = 0
			session.ToolChoice = "auto"
			for _, tc := range resp.ToolCalls {
				a.emitProgress(AnalysisProgressEvent{Type: "tool_call", Turn: turn, Tool: tc.Name, Path: extractToolPath(tc), Input: formatJSONForProgress(tc.Arguments)})
			}

			// Append assistant message with tool_calls to history BEFORE executing tools.
			session.Messages = append(session.Messages, ConversationMessage{
				Role:             "assistant",
				Content:          content,   // may be empty when only tool calls
				ReasoningContent: reasoning, // must be passed back for thinking-mode models
				ToolCalls:        resp.ToolCalls,
			})

			toolResults, newRecs, finished, toolErr := executeToolCalls(ctx, resp.ToolCalls, scanDeeper, checkDirContent, cfg, func(name, path, input, output string) {
				a.emitProgress(AnalysisProgressEvent{Type: "tool_result", Turn: turn, Tool: name, Path: path, Input: truncateProgressText(input, 12000), Output: truncateProgressText(output, 12000)})
			})

			// Append one tool-role message per tool call result.
			for _, tr := range toolResults {
				session.Messages = append(session.Messages, ConversationMessage{
					Role:       "tool",
					ToolCallID: tr.ID,
					Name:       tr.Name,
					Content:    tr.Output,
				})
			}

			if toolErr != nil {
				if strings.Contains(strings.ToLower(toolErr.Error()), "submit_recommendations arguments invalid") {
					if parsed, parseErr := ParseRecommendations(content); parseErr == nil {
						parsed = resolveRecommendationSizes(parsed, scanDeeper)
						if normalizeUsage(analysisUsage).TotalTokens == 0 {
							analysisUsage = estimateUsageFromTexts(session.System, content)
						}
						a.recordUsage(analysisUsage)
						a.recordDebug(strings.Join(session.RawOutputs, "\n\n"), "", "llm")
						a.emitProgress(AnalysisProgressEvent{Type: "completed", Turn: turn, Content: "分析完成（回退为直接解析）"})
						return parsed, nil
					}
					session.ToolChoice = "required"
					appendUserNote("submit_recommendations 参数格式无效。arguments 必须是严格 JSON 对象：{\"recommendations\":[...] }，不要在参数中混入解释文字。")
					session.NextTurn++
					continue
				}
				appendUserNote("工具调用异常（请修正参数后继续）：\n" + toolErr.Error())
				session.NextTurn++
				continue
			}

			// Accumulate any newly submitted recommendations.
			if len(newRecs) > 0 {
				session.AccumulatedRecs = append(session.AccumulatedRecs, newRecs...)
			}
			// finish_analysis signals the model is done.
			if finished {
				allRecs := resolveRecommendationSizes(session.AccumulatedRecs, scanDeeper)
				if normalizeUsage(analysisUsage).TotalTokens == 0 {
					analysisUsage = estimateUsageFromTexts(session.System, recommendationsAsText(allRecs))
				}
				a.recordUsage(analysisUsage)
				a.recordDebug(strings.Join(session.RawOutputs, "\n\n"), "", "llm")
				a.emitProgress(AnalysisProgressEvent{Type: "completed", Turn: turn, Content: "分析完成"})
				return allRecs, nil
			}
			if len(toolResults) > 0 {
				// Tool results are already in session.Messages; just advance turn.
				session.NextTurn++
				continue
			}
			session.NonCompliantTurns++
			session.ToolChoice = "required"
			appendUserNote("你返回了工具调用，但参数无效。请严格按工具 schema 重新调用，或直接输出有效 JSON 数组。")
			session.NextTurn++
			continue
		}

		// --- PATH 3: content only, try to parse as JSON recommendations ---
		if content != "" {
			// Append the assistant message to history regardless.
			session.Messages = append(session.Messages, ConversationMessage{
				Role:             "assistant",
				Content:          content,
				ReasoningContent: reasoning,
			})

			if parsed, parseErr := ParseRecommendations(content); parseErr == nil {
				parsed = resolveRecommendationSizes(parsed, scanDeeper)
				if normalizeUsage(analysisUsage).TotalTokens == 0 {
					analysisUsage = estimateUsageFromTexts(session.System, content)
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
					appendUserNote("系统自动补充深扫结果：\n" + strings.Join(probes, "\n\n"))
				}
			}
			appendUserNote(
				"上轮输出不是有效 JSON 数组，也未提供可执行 tool_call。你必须二选一：\n" +
					"1) 调用 scan_deeper/check_dir_content 然后调用 submit_recommendations；\n" +
					"2) 直接输出 JSON 数组，且不要任何解释文字。",
			)
			session.NextTurn++
			continue
		}

		// --- PATH 4: empty response ---
		session.NonCompliantTurns++
		session.ToolChoice = "required"
		if !session.AutoProbeDone && session.NonCompliantTurns >= 2 {
			session.AutoProbeDone = true
			if probes := autoProbeLargeDirs(session.Root, scanDeeper); len(probes) > 0 {
				appendUserNote("系统自动补充深扫结果：\n" + strings.Join(probes, "\n\n"))
			}
		}
		appendUserNote("你尚未给出可解析输出。请直接返回 JSON 数组或 tool_call。")
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

func isResumableAnalysisError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, ErrRateLimitPaused) {
		return true
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}

	msg := strings.ToLower(err.Error())
	retryableHints := []string{
		"timeout",
		"deadline exceeded",
		"network",
		"connection reset",
		"connection refused",
		"temporary",
		"service unavailable",
		"gateway timeout",
		"status: 503",
		"status=503",
		"status code: 503",
	}
	for _, hint := range retryableHints {
		if strings.Contains(msg, hint) {
			return true
		}
	}

	return false
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
	if text == "" || maxLen <= 0 {
		return text
	}
	if len(text) <= maxLen {
		return text
	}
	return text[:maxLen] + "\n...(截断)"
}

func buildTurnUserPrompt(base string, notes []string) string {
	if len(notes) == 0 {
		return base
	}
	return base + "\n\n会话上下文（按时间顺序）：\n" + strings.Join(notes, "\n\n")
}

func interruptedStreamingNote(turn int, content string) string {
	content = strings.TrimSpace(content)
	if content == "" {
		return ""
	}

	parts := []string{
		fmt.Sprintf("上一轮（turn %d）流式输出在中断前已经收到部分内容。不要重复这些内容，请基于它们继续完成当前分析。", turn),
	}
	if content != "" {
		parts = append(parts, "已收到的回答片段：\n"+content)
	}
	return strings.Join(parts, "\n\n")
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

type TavilyConfig struct {
	APIKey      string
	BaseURL     string
	Query       string
	SearchDepth string
	MaxResults  int
}

// toolCallResult groups a single tool execution outcome.
type toolCallResult struct {
	ID     string // synthetic tool_call_id, matches the assistant message's tool_calls entry
	Name   string
	Output string
}

func executeToolCalls(ctx context.Context, toolCalls []ToolCall, scanDeeper func(path string, depth int) (models.DirNode, error), checkDirContent func(path string) (models.FileTypeDistribution, error), cfg models.LLMConfig, onExecuted func(name, path, input, output string)) ([]toolCallResult, []models.Recommendation, bool, error) {
	tcResults := make([]toolCallResult, 0, len(toolCalls))
	var accumulatedRecs []models.Recommendation
	finished := false
	for i, tc := range toolCalls {
		callID := fmt.Sprintf("call_%d", i)
		switch tc.Name {
		case ToolFinishAnalysis:
			if onExecuted != nil {
				onExecuted(tc.Name, "", "{}", "finish_analysis received, terminating analysis loop")
			}
			finished = true
			tcResults = append(tcResults, toolCallResult{ID: callID, Name: tc.Name, Output: "finish_analysis acknowledged"})
		case ToolSubmitRecommendations:
			recs, err := parseSubmitRecommendations(tc.Arguments)
			if err != nil {
				if onExecuted != nil {
					onExecuted(tc.Name, "", formatJSONForProgress(tc.Arguments), "submit_recommendations error: "+err.Error())
				}
				return nil, nil, false, fmt.Errorf("submit_recommendations arguments invalid: %w", err)
			}
			if len(recs) > 0 {
				msg := fmt.Sprintf("submit_recommendations: accepted %d item(s). You may continue submitting more or call finish_analysis when done.", len(recs))
				if onExecuted != nil {
					onExecuted(tc.Name, "", formatJSONForProgress(tc.Arguments), fmt.Sprintf("submit_recommendations accepted %d item(s), analysis continues", len(recs)))
				}
				accumulatedRecs = append(accumulatedRecs, recs...)
				tcResults = append(tcResults, toolCallResult{ID: callID, Name: tc.Name, Output: msg})
				continue
			}
			emptyMsg := "submit_recommendations returned empty recommendations"
			if onExecuted != nil {
				onExecuted(tc.Name, "", formatJSONForProgress(tc.Arguments), emptyMsg)
			}
			tcResults = append(tcResults, toolCallResult{ID: callID, Name: tc.Name, Output: emptyMsg})
		case ToolScanDeeper:
			if scanDeeper == nil {
				unavailable := "scan_deeper unavailable on current runtime"
				if onExecuted != nil {
					onExecuted(tc.Name, extractToolPath(tc), formatJSONForProgress(tc.Arguments), unavailable)
				}
				tcResults = append(tcResults, toolCallResult{ID: callID, Name: tc.Name, Output: "scan_deeper error:\n" + unavailable})
				continue
			}
			var args struct {
				Path  string `json:"path"`
				Depth int    `json:"depth"`
			}
			if err := json.Unmarshal(tc.Arguments, &args); err != nil {
				msg := fmt.Sprintf("scan_deeper arguments invalid: %v", err)
				if onExecuted != nil {
					onExecuted(tc.Name, "", formatJSONForProgress(tc.Arguments), msg)
				}
				tcResults = append(tcResults, toolCallResult{ID: callID, Name: tc.Name, Output: "scan_deeper error:\n" + msg})
				continue
			}
			args.Path = strings.TrimSpace(args.Path)
			if args.Path == "" {
				msg := "scan_deeper arguments invalid: path is empty"
				if onExecuted != nil {
					onExecuted(tc.Name, "", formatJSONForProgress(tc.Arguments), msg)
				}
				tcResults = append(tcResults, toolCallResult{ID: callID, Name: tc.Name, Output: "scan_deeper error:\n" + msg})
				continue
			}
			if args.Depth <= 0 {
				args.Depth = 3
			}
			child, err := scanDeeper(args.Path, args.Depth)
			if err != nil {
				msg := fmt.Sprintf("scan_deeper failed: %v", err)
				if onExecuted != nil {
					onExecuted(tc.Name, args.Path, formatJSONForProgress(tc.Arguments), msg)
				}
				tcResults = append(tcResults, toolCallResult{ID: callID, Name: tc.Name, Output: "scan_deeper error:\n" + msg})
				continue
			}
			rendered := scanner.RenderCompressedTree(child, scanner.RenderConfig{TopNPerLevel: 20, MinChildSize: 10 * 1024 * 1024})
			if onExecuted != nil {
				onExecuted(tc.Name, args.Path, formatJSONForProgress(tc.Arguments), rendered)
			}
			tcResults = append(tcResults, toolCallResult{ID: callID, Name: tc.Name, Output: "scan_deeper result:\n" + rendered})
		case ToolCheckDirContent:
			if checkDirContent == nil {
				unavailable := "check_dir_content unavailable on current runtime"
				if onExecuted != nil {
					onExecuted(tc.Name, extractToolPath(tc), formatJSONForProgress(tc.Arguments), unavailable)
				}
				tcResults = append(tcResults, toolCallResult{ID: callID, Name: tc.Name, Output: "check_dir_content error:\n" + unavailable})
				continue
			}
			var args struct {
				Path string `json:"path"`
			}
			if err := json.Unmarshal(tc.Arguments, &args); err != nil {
				msg := fmt.Sprintf("check_dir_content arguments invalid: %v", err)
				if onExecuted != nil {
					onExecuted(tc.Name, "", formatJSONForProgress(tc.Arguments), msg)
				}
				tcResults = append(tcResults, toolCallResult{ID: callID, Name: tc.Name, Output: "check_dir_content error:\n" + msg})
				continue
			}
			args.Path = strings.TrimSpace(args.Path)
			if args.Path == "" {
				msg := "check_dir_content arguments invalid: path is empty"
				if onExecuted != nil {
					onExecuted(tc.Name, "", formatJSONForProgress(tc.Arguments), msg)
				}
				tcResults = append(tcResults, toolCallResult{ID: callID, Name: tc.Name, Output: "check_dir_content error:\n" + msg})
				continue
			}
			dist, err := checkDirContent(args.Path)
			if err != nil {
				msg := fmt.Sprintf("check_dir_content failed: %v", err)
				if onExecuted != nil {
					onExecuted(tc.Name, args.Path, formatJSONForProgress(tc.Arguments), msg)
				}
				tcResults = append(tcResults, toolCallResult{ID: callID, Name: tc.Name, Output: "check_dir_content error:\n" + msg})
				continue
			}
			blob, _ := json.MarshalIndent(dist, "", "  ")
			if onExecuted != nil {
				onExecuted(tc.Name, args.Path, formatJSONForProgress(tc.Arguments), string(blob))
			}
			tcResults = append(tcResults, toolCallResult{ID: callID, Name: tc.Name, Output: "check_dir_content result:\n" + string(blob)})
		case ToolTavilySearch:
			if !IsTavilySearchEnabled(cfg) {
				msg := "tavily_search is disabled (enable_web_search=true and non-empty tavily_api_key required)"
				if onExecuted != nil {
					onExecuted(tc.Name, "", formatJSONForProgress(tc.Arguments), msg)
				}
				tcResults = append(tcResults, toolCallResult{ID: callID, Name: tc.Name, Output: "tavily_search error:\n" + msg})
				continue
			}
			var args struct {
				Query       string `json:"query"`
				SearchDepth string `json:"search_depth"`
				MaxResults  int    `json:"max_results"`
			}
			if err := json.Unmarshal(tc.Arguments, &args); err != nil {
				msg := fmt.Sprintf("tavily_search arguments invalid: %v", err)
				if onExecuted != nil {
					onExecuted(tc.Name, "", formatJSONForProgress(tc.Arguments), msg)
				}
				tcResults = append(tcResults, toolCallResult{ID: callID, Name: tc.Name, Output: "tavily_search error:\n" + msg})
				continue
			}

			output, err := runTavilySearch(ctx, TavilyConfig{
				APIKey:      strings.TrimSpace(cfg.TavilyAPIKey),
				BaseURL:     strings.TrimSpace(cfg.TavilyBaseURL),
				Query:       strings.TrimSpace(args.Query),
				SearchDepth: strings.TrimSpace(args.SearchDepth),
				MaxResults:  args.MaxResults,
			})
			if err != nil {
				msg := fmt.Sprintf("tavily_search failed: %v", err)
				if onExecuted != nil {
					onExecuted(tc.Name, "", formatJSONForProgress(tc.Arguments), msg)
				}
				tcResults = append(tcResults, toolCallResult{ID: callID, Name: tc.Name, Output: "tavily_search error:\n" + msg})
				continue
			}
			if onExecuted != nil {
				onExecuted(tc.Name, "", formatJSONForProgress(tc.Arguments), output)
			}
			tcResults = append(tcResults, toolCallResult{ID: callID, Name: tc.Name, Output: "tavily_search result:\n" + output})
		default:
			msg := fmt.Sprintf("unsupported tool call: %s", tc.Name)
			if onExecuted != nil {
				onExecuted(tc.Name, extractToolPath(tc), formatJSONForProgress(tc.Arguments), msg)
			}
			tcResults = append(tcResults, toolCallResult{ID: callID, Name: tc.Name, Output: "tool error:\n" + msg})
		}
	}

	return tcResults, accumulatedRecs, finished, nil
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
