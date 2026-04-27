package analyzer

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"sort"
	"strings"
	"time"

	"disksage/internal/models"
)

const (
	defaultLLMTimeout = 120 * time.Second
	retryDelay        = 400 * time.Millisecond
)

type OpenAICompatibleClient struct {
	httpClient *http.Client
}

func NewOpenAICompatibleClient() *OpenAICompatibleClient {
	return &OpenAICompatibleClient{
		httpClient: &http.Client{Timeout: defaultLLMTimeout},
	}
}

func (c *OpenAICompatibleClient) Complete(ctx context.Context, req LLMRequest) (LLMResponse, error) {
	if req.Config.APIKey == "" {
		return LLMResponse{}, fmt.Errorf("missing API key")
	}
	base := strings.TrimRight(req.Config.BaseURL, "/")
	if base == "" {
		return LLMResponse{}, fmt.Errorf("missing base url")
	}

	body := map[string]any{
		"model": req.Config.Model,
		"messages": []map[string]string{
			{"role": "system", "content": req.System},
			{"role": "user", "content": req.User},
		},
	}
	if req.Config.EnableStreaming {
		body["stream"] = true
	}
	if len(req.Tools) > 0 {
		body["tools"] = toOpenAITools(req.Tools)
		choice := strings.TrimSpace(req.ToolChoice)
		if choice == "" {
			choice = "auto"
		}
		body["tool_choice"] = choice
	}
	if req.Config.MaxTokens > 0 {
		body["max_tokens"] = req.Config.MaxTokens
	}
	payload, _ := json.Marshal(body)
	timeout := resolveLLMTimeout(req.Config)

	raw, statusCode, err := c.doChatCompletion(ctx, base+"/chat/completions", req.Config.APIKey, payload, timeout, req.Config.EnableStreaming, req.OnStreamDelta)
	if err != nil {
		if isTimeoutErr(err) {
			return LLMResponse{}, fmt.Errorf("llm request timeout after %s, please check network/base_url: %w", timeout.String(), err)
		}
		return LLMResponse{}, err
	}

	if statusCode >= 300 {
		return LLMResponse{}, fmt.Errorf("llm api failed: %s", strings.TrimSpace(string(raw)))
	}

	var decoded map[string]any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		return LLMResponse{}, err
	}

	choices := asSlice(decoded["choices"])
	if len(choices) == 0 {
		return LLMResponse{}, fmt.Errorf("empty llm response")
	}
	firstChoice := asMap(choices[0])
	message := asMap(firstChoice["message"])

	content := extractContent(message["content"])
	reasoning := extractReasoning(message)
	toolCalls := extractToolCalls(message)
	usage := extractUsage(decoded["usage"])

	if len(toolCalls) > 0 {
		return LLMResponse{ToolCalls: toolCalls, Content: content, Reasoning: reasoning, RawResponse: string(raw), Usage: usage}, nil
	}
	recs, err := ParseRecommendations(content)
	if err == nil {
		return LLMResponse{Recommendations: recs, Content: content, Reasoning: reasoning, RawResponse: string(raw), Usage: usage}, nil
	}
	return LLMResponse{Content: content, Reasoning: reasoning, RawResponse: string(raw), Usage: usage}, nil
}

func asMap(v any) map[string]any {
	m, ok := v.(map[string]any)
	if !ok {
		return nil
	}
	return m
}

func asSlice(v any) []any {
	s, ok := v.([]any)
	if !ok {
		return nil
	}
	return s
}

func asString(v any) string {
	s, ok := v.(string)
	if !ok {
		return ""
	}
	return s
}

func asInt(v any) int {
	switch n := v.(type) {
	case float64:
		return int(n)
	case int:
		return n
	case int64:
		return int(n)
	case json.Number:
		i, _ := n.Int64()
		return int(i)
	default:
		return 0
	}
}

func extractContent(v any) string {
	switch content := v.(type) {
	case string:
		return content
	case map[string]any:
		if txt := strings.TrimSpace(asString(content["text"])); txt != "" {
			return txt
		}
		if txt := strings.TrimSpace(asString(content["content"])); txt != "" {
			return txt
		}
		if txt := strings.TrimSpace(asString(content["reasoning"])); txt != "" {
			return txt
		}
		if txt := strings.TrimSpace(asString(content["reasoning_content"])); txt != "" {
			return txt
		}
		return ""
	case []any:
		parts := make([]string, 0, len(content))
		for _, part := range content {
			switch p := part.(type) {
			case string:
				if strings.TrimSpace(p) != "" {
					parts = append(parts, p)
				}
			case map[string]any:
				if txt := strings.TrimSpace(asString(p["text"])); txt != "" {
					parts = append(parts, txt)
					continue
				}
				if txt := strings.TrimSpace(asString(p["content"])); txt != "" {
					parts = append(parts, txt)
				}
			}
		}
		return strings.Join(parts, "\n")
	default:
		return ""
	}
}

func extractReasoning(message map[string]any) string {
	parts := make([]string, 0, 8)
	parts = appendUniqueText(parts, extractContent(message["reasoning"]))
	parts = appendUniqueText(parts, extractContent(message["reasoning_content"]))

	for _, part := range asSlice(message["content"]) {
		p := asMap(part)
		if len(p) == 0 {
			continue
		}

		partType := strings.ToLower(strings.TrimSpace(asString(p["type"])))
		if partType == "reasoning" || partType == "reasoning_content" || partType == "thinking" {
			parts = appendUniqueText(parts, extractContent(p["text"]))
			parts = appendUniqueText(parts, extractContent(p["content"]))
			parts = appendUniqueText(parts, extractContent(p["reasoning"]))
			parts = appendUniqueText(parts, extractContent(p["reasoning_content"]))
			parts = appendUniqueText(parts, extractContent(p["thinking"]))
		}

		parts = appendUniqueText(parts, extractContent(p["reasoning"]))
		parts = appendUniqueText(parts, extractContent(p["reasoning_content"]))
	}

	return strings.Join(parts, "\n")
}

func appendUniqueText(parts []string, text string) []string {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return parts
	}
	for _, existing := range parts {
		if existing == trimmed {
			return parts
		}
	}
	return append(parts, trimmed)
}

func extractToolCalls(message map[string]any) []ToolCall {
	result := make([]ToolCall, 0, 4)
	for _, item := range asSlice(message["tool_calls"]) {
		if tc, ok := parseToolCall(item); ok {
			result = append(result, tc)
		}
	}

	if len(result) == 0 {
		if tc, ok := parseToolCall(message["function_call"]); ok {
			result = append(result, tc)
		}
	}

	return result
}

func parseToolCall(v any) (ToolCall, bool) {
	call := asMap(v)
	if len(call) == 0 {
		return ToolCall{}, false
	}

	payload := call
	if fn := asMap(call["function"]); len(fn) > 0 {
		payload = fn
	}

	name := strings.TrimSpace(asString(payload["name"]))
	if name == "" {
		return ToolCall{}, false
	}

	return ToolCall{
		Name:      name,
		Arguments: normalizeToolArguments(payload["arguments"]),
	}, true
}

func normalizeToolArguments(v any) json.RawMessage {
	switch args := v.(type) {
	case nil:
		return json.RawMessage([]byte(`{}`))
	case string:
		raw := strings.TrimSpace(args)
		if raw == "" {
			return json.RawMessage([]byte(`{}`))
		}
		return json.RawMessage([]byte(raw))
	default:
		blob, err := json.Marshal(args)
		if err != nil || len(blob) == 0 {
			return json.RawMessage([]byte(`{}`))
		}
		return json.RawMessage(blob)
	}
}

func extractUsage(v any) models.TokenUsage {
	usageMap := asMap(v)
	in := asInt(usageMap["prompt_tokens"])
	out := asInt(usageMap["completion_tokens"])
	total := asInt(usageMap["total_tokens"])

	if in == 0 {
		in = asInt(usageMap["input_tokens"])
	}
	if out == 0 {
		out = asInt(usageMap["output_tokens"])
	}
	if total == 0 {
		total = in + out
	}

	return models.TokenUsage{
		InputTokens:  in,
		OutputTokens: out,
		TotalTokens:  total,
	}
}

func toOpenAITools(defs []ToolDefinition) []map[string]any {
	tools := make([]map[string]any, 0, len(defs))
	for _, d := range defs {
		tools = append(tools, map[string]any{
			"type": "function",
			"function": map[string]any{
				"name":        d.Name,
				"description": d.Description,
				"parameters":  d.Parameters,
			},
		})
	}
	return tools
}

func resolveLLMTimeout(cfg models.LLMConfig) time.Duration {
	if cfg.RequestTimeoutSeconds <= 0 {
		return defaultLLMTimeout
	}
	return time.Duration(cfg.RequestTimeoutSeconds) * time.Second
}

func isTimeoutErr(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	var netErr net.Error
	if errors.As(err, &netErr) {
		return netErr.Timeout()
	}
	return strings.Contains(strings.ToLower(err.Error()), "timeout")
}

func (c *OpenAICompatibleClient) doChatCompletion(ctx context.Context, url, apiKey string, payload []byte, timeout time.Duration, stream bool, onStreamDelta func(delta LLMStreamDelta)) ([]byte, int, error) {
	var lastErr error
	for attempt := 0; attempt < 2; attempt++ {
		httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
		if err != nil {
			return nil, 0, err
		}
		httpReq.Header.Set("Content-Type", "application/json")
		httpReq.Header.Set("Authorization", "Bearer "+apiKey)

		httpClient := *c.httpClient
		httpClient.Timeout = timeout

		resp, err := httpClient.Do(httpReq)
		if err != nil {
			lastErr = err
			if attempt == 0 && isRetryableNetErr(err) {
				time.Sleep(retryDelay)
				continue
			}
			return nil, 0, err
		}

		if stream && resp.StatusCode < 300 {
			raw, readErr := parseStreamedChatCompletion(resp.Body, onStreamDelta)
			_ = resp.Body.Close()
			if readErr != nil {
				return nil, 0, readErr
			}

			return raw, resp.StatusCode, nil
		}

		raw, readErr := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		if readErr != nil {
			return nil, 0, readErr
		}

		if attempt == 0 && isRetryableStatus(resp.StatusCode) {
			lastErr = fmt.Errorf("retryable status: %d", resp.StatusCode)
			time.Sleep(retryDelay)
			continue
		}

		return raw, resp.StatusCode, nil
	}

	if lastErr != nil {
		return nil, 0, lastErr
	}
	return nil, 0, fmt.Errorf("llm request failed")
}

type streamedToolCallBuilder struct {
	Name       string
	Arguments  strings.Builder
	FirstOrder int
}

func parseStreamedChatCompletion(body io.Reader, onStreamDelta func(delta LLMStreamDelta)) ([]byte, error) {
	scanner := bufio.NewScanner(body)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	dataLines := make([]string, 0, 4)
	plainLines := make([]string, 0, 16)
	seenSSEData := false
	contentParts := make([]string, 0, 64)
	reasoningParts := make([]string, 0, 16)
	usage := map[string]any{}
	toolCallBuilders := make(map[int]*streamedToolCallBuilder)
	orderCounter := 0

	consumeDataBlock := func(block []string) error {
		if len(block) == 0 {
			return nil
		}

		payload := strings.TrimSpace(strings.Join(block, "\n"))
		if payload == "" || payload == "[DONE]" {
			return nil
		}

		var chunk map[string]any
		if err := json.Unmarshal([]byte(payload), &chunk); err != nil {
			return fmt.Errorf("parse stream chunk failed: %w", err)
		}

		if u := asMap(chunk["usage"]); len(u) > 0 {
			for k, v := range u {
				usage[k] = v
			}
		}

		for _, choiceAny := range asSlice(chunk["choices"]) {
			choice := asMap(choiceAny)
			delta := asMap(choice["delta"])
			if len(delta) == 0 {
				delta = asMap(choice["message"])
			}
			if len(delta) == 0 {
				continue
			}

			if piece := extractContent(delta["content"]); piece != "" {
				contentParts = append(contentParts, piece)
				if onStreamDelta != nil {
					onStreamDelta(LLMStreamDelta{Content: piece})
				}
			}

			if reasoningPiece := extractContent(delta["reasoning"]); reasoningPiece != "" {
				reasoningParts = appendUniqueText(reasoningParts, reasoningPiece)
				if onStreamDelta != nil {
					onStreamDelta(LLMStreamDelta{Reasoning: reasoningPiece})
				}
			}
			if reasoningPiece := extractContent(delta["reasoning_content"]); reasoningPiece != "" {
				reasoningParts = appendUniqueText(reasoningParts, reasoningPiece)
				if onStreamDelta != nil {
					onStreamDelta(LLMStreamDelta{Reasoning: reasoningPiece})
				}
			}

			for _, part := range asSlice(delta["content"]) {
				p := asMap(part)
				if len(p) == 0 {
					continue
				}
				partType := strings.ToLower(strings.TrimSpace(asString(p["type"])))
				if partType == "reasoning" || partType == "reasoning_content" || partType == "thinking" {
					for _, candidate := range []string{
						extractContent(p["text"]),
						extractContent(p["content"]),
						extractContent(p["reasoning"]),
						extractContent(p["reasoning_content"]),
						extractContent(p["thinking"]),
					} {
						if candidate == "" {
							continue
						}
						reasoningParts = appendUniqueText(reasoningParts, candidate)
						if onStreamDelta != nil {
							onStreamDelta(LLMStreamDelta{Reasoning: candidate})
						}
					}
				}
			}

			for _, tcAny := range asSlice(delta["tool_calls"]) {
				tcMap := asMap(tcAny)
				if len(tcMap) == 0 {
					continue
				}

				idx := asInt(tcMap["index"])
				builder, exists := toolCallBuilders[idx]
				if !exists {
					builder = &streamedToolCallBuilder{FirstOrder: orderCounter}
					toolCallBuilders[idx] = builder
					orderCounter++
				}

				fn := asMap(tcMap["function"])
				if name := strings.TrimSpace(asString(fn["name"])); name != "" {
					builder.Name = name
				}
				if builder.Name == "" {
					if name := strings.TrimSpace(asString(tcMap["name"])); name != "" {
						builder.Name = name
					}
				}

				argValue := fn["arguments"]
				if argValue == nil {
					argValue = tcMap["arguments"]
				}
				switch args := argValue.(type) {
				case string:
					builder.Arguments.WriteString(args)
				case nil:
				default:
					blob, marshalErr := json.Marshal(args)
					if marshalErr == nil {
						builder.Arguments.Write(blob)
					}
				}
			}
		}

		return nil
	}

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			if err := consumeDataBlock(dataLines); err != nil {
				return nil, err
			}
			dataLines = dataLines[:0]
			continue
		}
		if strings.HasPrefix(line, ":") || strings.HasPrefix(line, "event:") {
			continue
		}
		if strings.HasPrefix(line, "data:") {
			seenSSEData = true
			dataLines = append(dataLines, strings.TrimSpace(strings.TrimPrefix(line, "data:")))
			continue
		}
		plainLines = append(plainLines, line)
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	if !seenSSEData {
		raw := strings.TrimSpace(strings.Join(plainLines, "\n"))
		if raw == "" {
			return nil, fmt.Errorf("empty streaming response")
		}
		return []byte(raw), nil
	}
	if err := consumeDataBlock(dataLines); err != nil {
		return nil, err
	}

	indexes := make([]int, 0, len(toolCallBuilders))
	for idx := range toolCallBuilders {
		indexes = append(indexes, idx)
	}
	sort.Ints(indexes)

	toolCalls := make([]ToolCall, 0, len(indexes))
	for _, idx := range indexes {
		builder := toolCallBuilders[idx]
		if strings.TrimSpace(builder.Name) == "" {
			continue
		}
		args := strings.TrimSpace(builder.Arguments.String())
		if args == "" {
			args = "{}"
		}
		toolCalls = append(toolCalls, ToolCall{
			Name:      builder.Name,
			Arguments: json.RawMessage([]byte(args)),
		})
	}

	message := map[string]any{
		"content": strings.Join(contentParts, ""),
	}
	if len(reasoningParts) > 0 {
		message["reasoning_content"] = strings.Join(reasoningParts, "\n")
	}
	if len(toolCalls) > 0 {
		items := make([]map[string]any, 0, len(toolCalls))
		for _, tc := range toolCalls {
			items = append(items, map[string]any{
				"type": "function",
				"function": map[string]any{
					"name":      tc.Name,
					"arguments": string(tc.Arguments),
				},
			})
		}
		message["tool_calls"] = items
	}

	finalPayload := map[string]any{
		"choices": []map[string]any{{
			"message": message,
		}},
	}
	if len(usage) > 0 {
		finalPayload["usage"] = usage
	}

	blob, err := json.Marshal(finalPayload)
	if err != nil {
		return nil, err
	}
	return blob, nil
}

func isRetryableNetErr(err error) bool {
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	var netErr net.Error
	if errors.As(err, &netErr) {
		return netErr.Timeout() || netErr.Temporary()
	}
	return false
}

func isRetryableStatus(statusCode int) bool {
	switch statusCode {
	case http.StatusRequestTimeout, http.StatusTooManyRequests, http.StatusBadGateway, http.StatusServiceUnavailable, http.StatusGatewayTimeout:
		return true
	default:
		return false
	}
}
