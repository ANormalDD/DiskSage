package models

import "testing"

func TestAppConfigValidateMaxTokens(t *testing.T) {
	cfg := DefaultAppConfig()
	cfg.LLM.MaxTokens = -1
	if err := cfg.Validate(); err != nil {
		t.Fatalf("expected -1 to be valid: %v", err)
	}

	cfg.LLM.MaxTokens = 0
	if err := cfg.Validate(); err == nil {
		t.Fatalf("expected 0 to be invalid")
	}

	cfg.LLM.MaxTokens = -2
	if err := cfg.Validate(); err == nil {
		t.Fatalf("expected values below -1 to be invalid")
	}
}

func TestAppConfigValidateMaxTurns(t *testing.T) {
	cfg := DefaultAppConfig()
	cfg.LLM.MaxTurns = -1
	if err := cfg.Validate(); err != nil {
		t.Fatalf("expected -1 to be valid: %v", err)
	}

	cfg.LLM.MaxTurns = 0
	if err := cfg.Validate(); err == nil {
		t.Fatalf("expected 0 to be invalid")
	}

	cfg.LLM.MaxTurns = -2
	if err := cfg.Validate(); err == nil {
		t.Fatalf("expected values below -1 to be invalid")
	}
}

func TestAppConfigValidateRequestTimeoutSeconds(t *testing.T) {
	cfg := DefaultAppConfig()
	cfg.LLM.RequestTimeoutSeconds = 1
	if err := cfg.Validate(); err != nil {
		t.Fatalf("expected positive timeout to be valid: %v", err)
	}

	cfg.LLM.RequestTimeoutSeconds = 0
	if err := cfg.Validate(); err == nil {
		t.Fatalf("expected 0 timeout to be invalid")
	}

	cfg.LLM.RequestTimeoutSeconds = -10
	if err := cfg.Validate(); err == nil {
		t.Fatalf("expected negative timeout to be invalid")
	}
}
