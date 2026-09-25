package config

import (
	"os"
	"testing"
)

func TestDefaultConfigEnvFallbacks(t *testing.T) {
	// 1. None set -> mock
	os.Unsetenv("GEMINI_API_KEY")
	os.Unsetenv("OPENAI_API_KEY")
	os.Unsetenv("ANTHROPIC_API_KEY")

	cfg := DefaultConfig()
	if cfg.LLM.Provider != "mock" {
		t.Errorf("expected provider 'mock' when no keys set, got %q", cfg.LLM.Provider)
	}

	// 2. OpenAI key set
	os.Setenv("OPENAI_API_KEY", "sk-openai-test")
	cfgOpenAI := DefaultConfig()
	if cfgOpenAI.LLM.Provider != "openai" {
		t.Errorf("expected provider 'openai', got %q", cfgOpenAI.LLM.Provider)
	}
	if cfgOpenAI.LLM.Model != "gpt-4o-mini" {
		t.Errorf("expected model 'gpt-4o-mini', got %q", cfgOpenAI.LLM.Model)
	}
	os.Unsetenv("OPENAI_API_KEY")

	// 3. Anthropic key set
	os.Setenv("ANTHROPIC_API_KEY", "sk-ant-test")
	cfgAnt := DefaultConfig()
	if cfgAnt.LLM.Provider != "anthropic" {
		t.Errorf("expected provider 'anthropic', got %q", cfgAnt.LLM.Provider)
	}
	if cfgAnt.LLM.Model != "claude-3-5-haiku-latest" {
		t.Errorf("expected model 'claude-3-5-haiku-latest', got %q", cfgAnt.LLM.Model)
	}
	os.Unsetenv("ANTHROPIC_API_KEY")

	// 4. Gemini key set (highest priority)
	os.Setenv("GEMINI_API_KEY", "gemini-test-key")
	cfgGem := DefaultConfig()
	if cfgGem.LLM.Provider != "gemini" {
		t.Errorf("expected provider 'gemini', got %q", cfgGem.LLM.Provider)
	}
	if cfgGem.LLM.Model != "gemini-2.0-flash" {
		t.Errorf("expected model 'gemini-2.0-flash', got %q", cfgGem.LLM.Model)
	}
	os.Unsetenv("GEMINI_API_KEY")
}
