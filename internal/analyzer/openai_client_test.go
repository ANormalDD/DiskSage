package analyzer

import (
	"context"
	"encoding/json"
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
