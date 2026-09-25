package reconciler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Fhuss/syl/pkg/card"
	"github.com/Fhuss/syl/pkg/watcher"
)

func TestExtractJSON(t *testing.T) {
	cases := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "clean json",
			input:    `{"operations": []}`,
			expected: `{"operations": []}`,
		},
		{
			name: "markdown fences",
			input: "```json\n{\n  \"operations\": [\n    {\"op\": \"ADD\", \"card_id\": \"c1\"}\n  ]\n}\n```",
			expected: "{\n  \"operations\": [\n    {\"op\": \"ADD\", \"card_id\": \"c1\"}\n  ]\n}",
		},
		{
			name: "conversational wrapper with code fence",
			input: "Here is your requested output:\n```json\n{\"operations\": [{\"op\": \"NOOP\"}]}\n```\nLet me know if you need more help!",
			expected: "{\"operations\": [{\"op\": \"NOOP\"}]}",
		},
		{
			name:     "empty input",
			input:    "   \n",
			expected: "",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := extractJSON(tc.input)
			if got != tc.expected {
				t.Errorf("extractJSON() = %q, expected %q", got, tc.expected)
			}
		})
	}
}

func TestLLMReconcilerOpenAI(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-openai-key" {
			t.Errorf("expected Bearer token test-openai-key, got %s", r.Header.Get("Authorization"))
		}
		if r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("expected Content-Type application/json")
		}

		resp := openAIResponse{
			Choices: []openAIChoice{
				{
					Message: openAIMessage{
						Role: "assistant",
						// Test returning markdown-wrapped json
						Content: "```json\n{\"operations\": [{\"op\": \"ADD\", \"card_id\": \"card-1\", \"title\": \"AWS S3\", \"content\": \"aws s3 ls\", \"reason\": \"New cmd\"}]}\n```",
					},
				},
			},
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	rec := NewLLMReconciler(LLMConfig{
		Provider: "openai",
		Endpoint: server.URL,
		APIKey:   "test-openai-key",
		Model:    "gpt-4o-mini",
		Timeout:  2 * time.Second,
	})

	muts, err := rec.Reconcile(context.Background(), nil, &watcher.Turn{
		ID:        1,
		User:      "List S3 buckets",
		Assistant: "Run aws s3 ls",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(muts) != 1 {
		t.Fatalf("expected 1 mutation, got %d", len(muts))
	}
	if muts[0].Op != OpAdd || muts[0].CardID != "card-1" || muts[0].Title != "AWS S3" {
		t.Errorf("unexpected mutation: %+v", muts[0])
	}
}

func TestLLMReconcilerAnthropic(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("x-api-key") != "test-ant-key" {
			t.Errorf("expected x-api-key test-ant-key, got %s", r.Header.Get("x-api-key"))
		}
		if r.Header.Get("anthropic-version") != "2023-06-01" {
			t.Errorf("expected anthropic-version 2023-06-01, got %s", r.Header.Get("anthropic-version"))
		}

		resp := anthropicResponse{
			Content: []anthropicContentBlock{
				{
					Type: "text",
					Text: "{\"operations\": [{\"op\": \"UPDATE\", \"card_id\": \"card-1\", \"content\": \"aws s3 sync . s3://bucket\", \"reason\": \"Updated command\"}]}",
				},
			},
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	rec := NewLLMReconciler(LLMConfig{
		Provider: "anthropic",
		Endpoint: server.URL,
		APIKey:   "test-ant-key",
		Model:    "claude-3-5-haiku-latest",
		Timeout:  2 * time.Second,
	})

	existing := []*card.Card{
		{
			ID:      "card-1",
			Title:   "AWS S3",
			Content: "aws s3 ls",
		},
	}

	muts, err := rec.Reconcile(context.Background(), existing, &watcher.Turn{
		ID:        2,
		User:      "sync to bucket",
		Assistant: "aws s3 sync . s3://bucket",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(muts) != 1 {
		t.Fatalf("expected 1 mutation, got %d", len(muts))
	}
	if muts[0].Op != OpUpdate || muts[0].CardID != "card-1" {
		t.Errorf("unexpected mutation: %+v", muts[0])
	}
}

func TestLLMReconcilerEndpointNormalization(t *testing.T) {
	// Base URL without /chat/completions
	rec := NewLLMReconciler(LLMConfig{
		Provider: "ollama",
		Endpoint: "http://localhost:11434/v1",
	})

	if rec.Config().Endpoint != "http://localhost:11434/v1/chat/completions" {
		t.Errorf("expected normalized endpoint, got %s", rec.Config().Endpoint)
	}
	if rec.Config().Model != "llama3.2" {
		t.Errorf("expected default model llama3.2, got %s", rec.Config().Model)
	}

	// Anthropic defaults
	recAnt := NewLLMReconciler(LLMConfig{
		Provider: "anthropic",
	})
	if recAnt.Config().Endpoint != "https://api.anthropic.com/v1/messages" {
		t.Errorf("expected Anthropic messages endpoint, got %s", recAnt.Config().Endpoint)
	}
	if recAnt.Config().Model != "claude-3-5-haiku-latest" {
		t.Errorf("expected default Anthropic model claude-3-5-haiku-latest, got %s", recAnt.Config().Model)
	}

	// Gemini defaults
	recGem := NewLLMReconciler(LLMConfig{
		Provider: "gemini",
	})
	if recGem.Config().Model != "gemini-2.0-flash" {
		t.Errorf("expected default Gemini model gemini-2.0-flash, got %s", recGem.Config().Model)
	}
}
