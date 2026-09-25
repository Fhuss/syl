package reconciler

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/Fhuss/syl/pkg/card"
	"github.com/Fhuss/syl/pkg/watcher"
)

type LLMConfig struct {
	Endpoint string        `yaml:"endpoint"`
	APIKey   string        `yaml:"api_key"`
	Model    string        `yaml:"model"`
	Timeout  time.Duration `yaml:"timeout"`
}

type LLMReconciler struct {
	cfg        LLMConfig
	httpClient *http.Client
}

func NewLLMReconciler(cfg LLMConfig) *LLMReconciler {
	if cfg.Timeout == 0 {
		cfg.Timeout = 15 * time.Second
	}
	if cfg.Endpoint == "" {
		cfg.Endpoint = "https://api.openai.com/v1/chat/completions"
	}
	if cfg.Model == "" {
		cfg.Model = "gpt-4o-mini"
	}

	return &LLMReconciler{
		cfg: cfg,
		httpClient: &http.Client{
			Timeout: cfg.Timeout,
		},
	}
}

type openAIMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type openAIRequest struct {
	Model          string          `json:"model"`
	Messages       []openAIMessage `json:"messages"`
	ResponseFormat *responseFormat `json:"response_format,omitempty"`
	Temperature    float32         `json:"temperature"`
}

type responseFormat struct {
	Type string `json:"type"`
}

type openAIChoice struct {
	Message openAIMessage `json:"message"`
}

type openAIResponse struct {
	Choices []openAIChoice `json:"choices"`
	Error   *struct {
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

type llmOutput struct {
	Operations []Mutation `json:"operations"`
}

func (l *LLMReconciler) Reconcile(ctx context.Context, currentCards []*card.Card, turn *watcher.Turn) ([]Mutation, error) {
	systemPrompt := `You are the reference reconciler for 'syl', an automated terminal reference pane that sits beside an AI conversation.
Your goal is to maintain accurate, concise reference cards (procedures, commands, architectural decisions) so the human operator doesn't lose track of active steps.

CRITICAL RULES:
1. IN-PLACE MUTATION OVER DUPLICATION: If this turn corrects, refines, changes parameters (e.g. region, credentials, flag), or adds a step to an EXISTING card, emit an 'UPDATE' op targeting that card_id. Do NOT create a duplicate card!
2. INVALIDATION: If an earlier instruction or command was retracted, broke, or superseded, emit 'INVALIDATE' on that card_id with the reason.
3. EXTRACTION: If new actionable procedures, reusable shell snippets, or explicit decisions were introduced, emit 'ADD'.
4. NOISE REDUCTION: If this turn is pure explanation, conversational pleasantries, or questions with no concrete procedural value, emit a single 'NOOP' op.
5. Content must be high-density Markdown (steps, clean code blocks).

OUTPUT JSON SCHEMA:
{
  "operations": [
    {
      "op": "ADD" | "UPDATE" | "INVALIDATE" | "NOOP",
      "card_id": "unique-slug-or-existing-id",
      "type": "PROCEDURE" | "COMMAND" | "DECISION" | "QUESTION" | "INFO",
      "title": "Short Descriptive Title",
      "content": "Clean markdown with code fences",
      "reason": "Why this operation was performed"
    }
  ]
}`

	cardsJSON, _ := json.MarshalIndent(currentCards, "", "  ")
	userPrompt := fmt.Sprintf("CURRENT REFERENCE CARDS:\n%s\n\nNEW CONVERSATION TURN:\n[User]:\n%s\n\n[Assistant]:\n%s\n\nReturn JSON operations:",
		string(cardsJSON), turn.User, turn.Assistant)

	reqBody := openAIRequest{
		Model: l.cfg.Model,
		Messages: []openAIMessage{
			{Role: "system", Content: systemPrompt},
			{Role: "user", Content: userPrompt},
		},
		ResponseFormat: &responseFormat{Type: "json_object"},
		Temperature:    0.1,
	}

	payload, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", l.cfg.Endpoint, bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if l.cfg.APIKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+l.cfg.APIKey)
	}

	resp, err := l.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("call llm api: %w", err)
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("llm api error (status %d): %s", resp.StatusCode, string(bodyBytes))
	}

	var openAIResp openAIResponse
	if err := json.Unmarshal(bodyBytes, &openAIResp); err != nil {
		return nil, fmt.Errorf("unmarshal response: %w", err)
	}

	if openAIResp.Error != nil {
		return nil, fmt.Errorf("llm api returned error: %s", openAIResp.Error.Message)
	}

	if len(openAIResp.Choices) == 0 {
		return nil, fmt.Errorf("no completion choices returned")
	}

	rawContent := strings.TrimSpace(openAIResp.Choices[0].Message.Content)
	var output llmOutput
	if err := json.Unmarshal([]byte(rawContent), &output); err != nil {
		return nil, fmt.Errorf("unmarshal operations json: %w (raw: %s)", err, rawContent)
	}

	return output.Operations, nil
}
