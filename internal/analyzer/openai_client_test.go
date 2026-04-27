package analyzer

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"disksage/internal/models"
)

type toolArgsPath struct {
	Path string `json:"path"`
}

func TestOpenAIClientInjectsTools(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("expected POST, got %s", r.Method)
		}
		body, _ := io.ReadAll(r.Body)
		_ = r.Body.Close()

		var payload map[string]any
		if err := json.Unmarshal(body, &payload); err != nil {
			t.Fatalf("invalid json payload: %v", err)
		}
		if _, ok := payload["tools"]; !ok {
			t.Fatalf("expected tools in payload")
		}
		if payload["tool_choice"] != "auto" {
			t.Fatalf("expected tool_choice auto")
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"[]"}}],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`))
	}))
	defer server.Close()

	client := NewOpenAICompatibleClient()
	_, err := client.Complete(context.Background(), LLMRequest{
		System: "sys",
		User:   "user",
		Tools:  DefaultToolDefinitions(),
		Config: models.LLMConfig{
			APIKey:  "k",
			BaseURL: server.URL,
			Model:   "m",
		},
	})
	if err != nil {
		t.Fatalf("complete failed: %v", err)
	}
}

func TestOpenAIClientParsesToolCalls(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
  "choices":[
    {
      "message":{
        "content":"",
        "tool_calls":[
          {
            "function":{
              "name":"scan_deeper",
              "arguments":"{\"path\":\"D:/temp\",\"depth\":3}"
            }
          }
        ]
      }
    }
  ],
  "usage":{"prompt_tokens":11,"completion_tokens":5,"total_tokens":16}
}`))
	}))
	defer server.Close()

	client := NewOpenAICompatibleClient()
	resp, err := client.Complete(context.Background(), LLMRequest{
		System: "sys",
		User:   "user",
		Tools:  DefaultToolDefinitions(),
		Config: models.LLMConfig{
			APIKey:  "k",
			BaseURL: server.URL,
			Model:   "m",
		},
	})
	if err != nil {
		t.Fatalf("complete failed: %v", err)
	}
	if len(resp.ToolCalls) != 1 {
		t.Fatalf("expected 1 tool call, got %d", len(resp.ToolCalls))
	}
	if resp.ToolCalls[0].Name != "scan_deeper" {
		t.Fatalf("unexpected tool call name: %s", resp.ToolCalls[0].Name)
	}
	if strings.TrimSpace(resp.RawResponse) == "" {
		t.Fatalf("expected raw response to be populated")
	}
	if !strings.Contains(string(resp.ToolCalls[0].Arguments), "depth") {
		t.Fatalf("unexpected tool args: %s", string(resp.ToolCalls[0].Arguments))
	}
}

func TestOpenAIClientParsesLegacyFunctionCall(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
  "choices":[
    {
      "message":{
        "content":"",
        "function_call":{
          "name":"check_dir_content",
          "arguments":"{\"path\":\"D:/Downloads\"}"
        }
      }
    }
  ],
  "usage":{"prompt_tokens":9,"completion_tokens":4,"total_tokens":13}
}`))
	}))
	defer server.Close()

	client := NewOpenAICompatibleClient()
	resp, err := client.Complete(context.Background(), LLMRequest{
		System: "sys",
		User:   "user",
		Tools:  DefaultToolDefinitions(),
		Config: models.LLMConfig{
			APIKey:  "k",
			BaseURL: server.URL,
			Model:   "m",
		},
	})
	if err != nil {
		t.Fatalf("complete failed: %v", err)
	}
	if len(resp.ToolCalls) != 1 {
		t.Fatalf("expected 1 tool call, got %d", len(resp.ToolCalls))
	}
	if resp.ToolCalls[0].Name != "check_dir_content" {
		t.Fatalf("unexpected tool call name: %s", resp.ToolCalls[0].Name)
	}
}

func TestOpenAIClientParsesToolCallsWithoutFunctionWrapper(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
  "choices":[
    {
      "message":{
        "content":"",
        "tool_calls":[
          {
            "name":"scan_deeper",
            "arguments":"{\"path\":\"D:/temp\",\"depth\":2}"
          }
        ]
      }
    }
  ],
  "usage":{"prompt_tokens":8,"completion_tokens":4,"total_tokens":12}
}`))
	}))
	defer server.Close()

	client := NewOpenAICompatibleClient()
	resp, err := client.Complete(context.Background(), LLMRequest{
		System: "sys",
		User:   "user",
		Tools:  DefaultToolDefinitions(),
		Config: models.LLMConfig{
			APIKey:  "k",
			BaseURL: server.URL,
			Model:   "m",
		},
	})
	if err != nil {
		t.Fatalf("complete failed: %v", err)
	}
	if len(resp.ToolCalls) != 1 {
		t.Fatalf("expected 1 tool call, got %d", len(resp.ToolCalls))
	}
	if resp.ToolCalls[0].Name != "scan_deeper" {
		t.Fatalf("unexpected tool call name: %s", resp.ToolCalls[0].Name)
	}
}

func TestOpenAIClientParsesToolCallsWithObjectArguments(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
  "choices":[
    {
      "message":{
        "content":[{"type":"output_text","text":""}],
        "tool_calls":[
          {
            "function":{
              "name":"check_dir_content",
              "arguments":{"path":"D:/Downloads"}
            }
          }
        ]
      }
    }
  ],
  "usage":{"input_tokens":7,"output_tokens":3}
}`))
	}))
	defer server.Close()

	client := NewOpenAICompatibleClient()
	resp, err := client.Complete(context.Background(), LLMRequest{
		System: "sys",
		User:   "user",
		Tools:  DefaultToolDefinitions(),
		Config: models.LLMConfig{
			APIKey:  "k",
			BaseURL: server.URL,
			Model:   "m",
		},
	})
	if err != nil {
		t.Fatalf("complete failed: %v", err)
	}
	if len(resp.ToolCalls) != 1 {
		t.Fatalf("expected 1 tool call, got %d", len(resp.ToolCalls))
	}
	if resp.ToolCalls[0].Name != "check_dir_content" {
		t.Fatalf("unexpected tool call name: %s", resp.ToolCalls[0].Name)
	}
	if !strings.Contains(string(resp.ToolCalls[0].Arguments), "path") {
		t.Fatalf("unexpected tool args: %s", string(resp.ToolCalls[0].Arguments))
	}
	if resp.Usage.InputTokens != 7 || resp.Usage.OutputTokens != 3 || resp.Usage.TotalTokens != 10 {
		t.Fatalf("unexpected usage: %+v", resp.Usage)
	}
}

func TestOpenAIClientParsesDeepSeekStyleToolCallPayload(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
  "id":"019d4bdea53fe2fcc67e1a9531f75258",
  "object":"chat.completion",
  "created":1775094375,
  "model":"deepseek-ai/DeepSeek-V3.2",
  "choices":[
    {
      "index":0,
      "message":{
        "role":"assistant",
        "content":"我将分析这个目录树并提供清理建议。",
        "tool_calls":[
          {
            "id":"019d4bdeb726e383e3f0f6375024b061",
            "type":"function",
            "function":{
              "name":"check_dir_content",
	              "arguments":"{\"path\": \"D:\\\\Clash Verge\\\\.config\\\\clash-verge\\\\logs\"}"
            }
          }
        ]
      },
      "finish_reason":"tool_calls"
    }
  ],
  "usage":{
    "prompt_tokens":7197,
    "completion_tokens":88,
    "total_tokens":7285
  }
}`))
	}))
	defer server.Close()

	client := NewOpenAICompatibleClient()
	resp, err := client.Complete(context.Background(), LLMRequest{
		System: "sys",
		User:   "user",
		Tools:  DefaultToolDefinitions(),
		Config: models.LLMConfig{
			APIKey:  "k",
			BaseURL: server.URL,
			Model:   "m",
		},
	})
	if err != nil {
		t.Fatalf("complete failed: %v", err)
	}
	if len(resp.ToolCalls) != 1 {
		t.Fatalf("expected 1 tool call, got %d", len(resp.ToolCalls))
	}
	if resp.ToolCalls[0].Name != ToolCheckDirContent {
		t.Fatalf("unexpected tool call name: %s", resp.ToolCalls[0].Name)
	}

	var args toolArgsPath
	if err := json.Unmarshal(resp.ToolCalls[0].Arguments, &args); err != nil {
		t.Fatalf("failed to decode tool args: %v; raw=%s", err, string(resp.ToolCalls[0].Arguments))
	}
	if args.Path != `D:\Clash Verge\.config\clash-verge\logs` {
		t.Fatalf("unexpected parsed path: %s", args.Path)
	}
}

func TestOpenAIClientExtractsReasoningContent(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
  "choices":[
    {
      "message":{
        "content":"",
        "reasoning_content":"先定位大目录，再调用工具校验内容。",
        "tool_calls":[
          {
            "function":{
              "name":"scan_deeper",
              "arguments":"{\"path\":\"D:/temp\",\"depth\":2}"
            }
          }
        ]
      }
    }
  ],
  "usage":{"prompt_tokens":6,"completion_tokens":2,"total_tokens":8}
}`))
	}))
	defer server.Close()

	client := NewOpenAICompatibleClient()
	resp, err := client.Complete(context.Background(), LLMRequest{
		System: "sys",
		User:   "user",
		Tools:  DefaultToolDefinitions(),
		Config: models.LLMConfig{
			APIKey:  "k",
			BaseURL: server.URL,
			Model:   "m",
		},
	})
	if err != nil {
		t.Fatalf("complete failed: %v", err)
	}
	if strings.TrimSpace(resp.Reasoning) == "" {
		t.Fatalf("expected reasoning to be extracted")
	}
	if !strings.Contains(resp.Reasoning, "先定位大目录") {
		t.Fatalf("unexpected reasoning: %s", resp.Reasoning)
	}
}

func TestOpenAIClientParsesStreamingResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		_ = r.Body.Close()

		var payload map[string]any
		if err := json.Unmarshal(body, &payload); err != nil {
			t.Fatalf("invalid json payload: %v", err)
		}
		if stream, _ := payload["stream"].(bool); !stream {
			t.Fatalf("expected stream=true in payload")
		}

		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"你好\"}}]}\n\n"))
		_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"，世界\"}}]}\n\n"))
		_, _ = w.Write([]byte("data: {\"usage\":{\"prompt_tokens\":9,\"completion_tokens\":3,\"total_tokens\":12}}\n\n"))
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
	}))
	defer server.Close()

	client := NewOpenAICompatibleClient()
	resp, err := client.Complete(context.Background(), LLMRequest{
		System: "sys",
		User:   "user",
		Config: models.LLMConfig{
			APIKey:          "k",
			BaseURL:         server.URL,
			Model:           "m",
			EnableStreaming: true,
		},
	})
	if err != nil {
		t.Fatalf("complete failed: %v", err)
	}
	if resp.Content != "你好，世界" {
		t.Fatalf("unexpected streamed content: %s", resp.Content)
	}
	if resp.Usage.TotalTokens != 12 {
		t.Fatalf("unexpected streamed usage: %+v", resp.Usage)
	}
}

func TestOpenAIClientStreamingFallbackToPlainJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
  "choices":[
    {
      "message":{
        "content":"[{\"path\":\"D:/tmp\",\"size\":1,\"category\":\"safe\",\"reason\":\"x\",\"clean_method\":\"recycle\",\"command\":\"\",\"risk\":\"low\"}]"
      }
    }
  ],
  "usage":{"prompt_tokens":5,"completion_tokens":3,"total_tokens":8}
}`))
	}))
	defer server.Close()

	client := NewOpenAICompatibleClient()
	resp, err := client.Complete(context.Background(), LLMRequest{
		System: "sys",
		User:   "user",
		Config: models.LLMConfig{
			APIKey:                "k",
			BaseURL:               server.URL,
			Model:                 "m",
			EnableStreaming:       true,
			RequestTimeoutSeconds: 120,
		},
	})
	if err != nil {
		t.Fatalf("complete failed: %v", err)
	}
	if len(resp.Recommendations) != 1 {
		t.Fatalf("expected 1 recommendation from plain JSON fallback, got %d", len(resp.Recommendations))
	}
}

func TestOpenAIClientParsesStreamingToolCallFragments(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")

		chunk1 := map[string]any{
			"choices": []any{map[string]any{
				"delta": map[string]any{
					"tool_calls": []any{map[string]any{
						"index": 0,
						"function": map[string]any{
							"name":      "scan_deeper",
							"arguments": `{"path":"D:/temp",`,
						},
					}},
				},
			}},
		}
		chunk2 := map[string]any{
			"choices": []any{map[string]any{
				"delta": map[string]any{
					"tool_calls": []any{map[string]any{
						"index": 0,
						"function": map[string]any{
							"arguments": `"depth":3}`,
						},
					}},
				},
			}},
		}

		blob1, _ := json.Marshal(chunk1)
		blob2, _ := json.Marshal(chunk2)
		_, _ = fmt.Fprintf(w, "data: %s\n\n", string(blob1))
		_, _ = fmt.Fprintf(w, "data: %s\n\n", string(blob2))
		_, _ = fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	defer server.Close()

	client := NewOpenAICompatibleClient()
	resp, err := client.Complete(context.Background(), LLMRequest{
		System: "sys",
		User:   "user",
		Config: models.LLMConfig{
			APIKey:                "k",
			BaseURL:               server.URL,
			Model:                 "m",
			EnableStreaming:       true,
			RequestTimeoutSeconds: 120,
		},
	})
	if err != nil {
		t.Fatalf("complete failed: %v", err)
	}
	if len(resp.ToolCalls) != 1 {
		t.Fatalf("expected 1 tool call, got %d", len(resp.ToolCalls))
	}
	if resp.ToolCalls[0].Name != ToolScanDeeper {
		t.Fatalf("unexpected tool name: %s", resp.ToolCalls[0].Name)
	}
	if !strings.Contains(string(resp.ToolCalls[0].Arguments), "depth") {
		t.Fatalf("expected fragmented arguments to merge, got: %s", string(resp.ToolCalls[0].Arguments))
	}
}

func TestOpenAIClientStreamingInvokesDeltaCallback(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"A\",\"reasoning_content\":\"R\"}}]}\n\n"))
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
	}))
	defer server.Close()

	client := NewOpenAICompatibleClient()
	deltaContent := ""
	deltaReasoning := ""
	_, err := client.Complete(context.Background(), LLMRequest{
		System: "sys",
		User:   "user",
		Config: models.LLMConfig{
			APIKey:                "k",
			BaseURL:               server.URL,
			Model:                 "m",
			EnableStreaming:       true,
			RequestTimeoutSeconds: 120,
		},
		OnStreamDelta: func(delta LLMStreamDelta) {
			deltaContent += delta.Content
			deltaReasoning += delta.Reasoning
		},
	})
	if err != nil {
		t.Fatalf("complete failed: %v", err)
	}
	if deltaContent != "A" {
		t.Fatalf("unexpected stream delta content: %s", deltaContent)
	}
	if deltaReasoning != "R" {
		t.Fatalf("unexpected stream delta reasoning: %s", deltaReasoning)
	}
}

func TestOpenAIClientStreamingPreservesWhitespaceChunks(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"A\"}}]}\n\n"))
		_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\" \"}}]}\n\n"))
		_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"B\"}}]}\n\n"))
		_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"\\n\"}}]}\n\n"))
		_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"C\"}}]}\n\n"))
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
	}))
	defer server.Close()

	client := NewOpenAICompatibleClient()
	deltaContent := ""
	resp, err := client.Complete(context.Background(), LLMRequest{
		System: "sys",
		User:   "user",
		Config: models.LLMConfig{
			APIKey:                "k",
			BaseURL:               server.URL,
			Model:                 "m",
			EnableStreaming:       true,
			RequestTimeoutSeconds: 120,
		},
		OnStreamDelta: func(delta LLMStreamDelta) {
			deltaContent += delta.Content
		},
	})
	if err != nil {
		t.Fatalf("complete failed: %v", err)
	}
	if deltaContent != "A B\nC" {
		t.Fatalf("unexpected stream delta content: %q", deltaContent)
	}
	if resp.Content != "A B\nC" {
		t.Fatalf("unexpected streamed content: %q", resp.Content)
	}
}
