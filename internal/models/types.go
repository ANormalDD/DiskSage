package models

import (
	"errors"
	"path/filepath"
	"strings"
	"time"
)

type ScanConfig struct {
	MaxDepth   int
	MinDirSize int64
	SkipDirs   []string
	TopN       int
}

func DefaultScanConfig() ScanConfig {
	return ScanConfig{
		MaxDepth:   4,
		MinDirSize: 50 * 1024 * 1024,
		SkipDirs: []string{
			"Windows",
			"$Recycle.Bin",
			"System Volume Information",
		},
		TopN: 20,
	}
}

type DirNode struct {
	Path         string
	Name         string
	Size         int64
	Children     []DirNode
	FileTypes    []FileTypeStat
	MarkerLabels []string
	IsFile       bool
	ModTime      time.Time
}

type FileTypeStat struct {
	Pattern string
	Count   int
	Size    int64
}

type FileTypeDistribution struct {
	Path  string
	Stats []FileTypeStat
}

type ScanProgress struct {
	Path      string
	DirsSeen  int64
	FilesSeen int64
	BytesSeen int64
	Done      bool
}

type ScanResult struct {
	Root       DirNode
	Compressed string
}

type Recommendation struct {
	Path        string        `json:"path"`
	Size        int64         `json:"size"`
	Category    CleanCategory `json:"category"`
	Reason      string        `json:"reason"`
	CleanMethod CleanMethod   `json:"clean_method"`
	Command     string        `json:"command"`
	Risk        string        `json:"risk"`
}

type CleanCategory string

const (
	CategorySafe    CleanCategory = "safe"
	CategoryConfirm CleanCategory = "confirm"
	CategoryManual  CleanCategory = "manual"
	CategoryReview  CleanCategory = "review"
)

type CleanMethod string

const (
	MethodDelete   CleanMethod = "delete"
	MethodCommand  CleanMethod = "command"
	MethodRecycle  CleanMethod = "recycle"
	MethodRedirect CleanMethod = "redirect"
)

type CleanRequest struct {
	Items           []Recommendation
	PermanentDelete bool
	ConfirmCommands bool
	RequestedBy     string
}

type ItemCleanResult struct {
	Path    string
	Success bool
	Error   string
	Freed   int64
}

type CleanSummary struct {
	StartedAt time.Time
	EndedAt   time.Time
	Results   []ItemCleanResult
	Freed     int64
}

type LLMConfig struct {
	Provider  string `json:"provider"`
	APIKey    string `json:"api_key"`
	Model     string `json:"model"`
	BaseURL   string `json:"base_url"`
	MaxTokens int    `json:"max_tokens"`
	MaxTurns  int    `json:"max_turns"`
}

type TokenUsage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
	TotalTokens  int `json:"total_tokens"`
}

type TokenStats struct {
	Last         TokenUsage `json:"last"`
	Total        TokenUsage `json:"total"`
	RequestCount int        `json:"request_count"`
}

type LLMDebugInfo struct {
	RawOutput string    `json:"raw_output"`
	LastError string    `json:"last_error"`
	Source    string    `json:"source"`
	UpdatedAt time.Time `json:"updated_at"`
}

type AppConfig struct {
	LLM LLMConfig `json:"llm"`
}

func DefaultAppConfig() AppConfig {
	return AppConfig{
		LLM: LLMConfig{
			Provider:  "openai",
			Model:     "gpt-4o-mini",
			BaseURL:   "https://api.openai.com/v1",
			MaxTokens: 1200,
			MaxTurns:  6,
		},
	}
}

func (c AppConfig) Validate() error {
	if c.LLM.MaxTokens == 0 || c.LLM.MaxTokens < -1 {
		return errors.New("max tokens must be positive or -1 (unlimited)")
	}
	if c.LLM.MaxTurns == 0 || c.LLM.MaxTurns < -1 {
		return errors.New("max turns must be positive or -1 (unlimited)")
	}
	if c.LLM.Provider == "" {
		return errors.New("provider cannot be empty")
	}
	if c.LLM.Model == "" {
		return errors.New("model cannot be empty")
	}
	return nil
}

func (r Recommendation) IsEmpty() bool {
	return r.Path == "" || r.Size <= 0
}

func NormalizePath(p string) string {
	cleaned := filepath.Clean(p)
	return strings.TrimSuffix(cleaned, string(filepath.Separator))
}
