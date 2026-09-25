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
	Provider string        `yaml:"provider"` // "openai" | "gemini" | "anthropic" | "ollama" | "mock"
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

	provider := strings.ToLower(strings.TrimSpace(cfg.Provider))
	if provider == "" {
		if strings.Contains(cfg.Endpoint, "anthropic.com") {
			provider = "anthropic"
		} else if strings.Contains(cfg.Endpoint, "googleapis.com") {
			provider = "gemini"
		} else if strings.Contains(cfg.Endpoint, "11434") {
			provider = "ollama"
		} else {
			provider = "openai"
		}
	}
	cfg.Provider = provider

	if cfg.Endpoint == "" {
		switch provider {
		case "anthropic":
			cfg.Endpoint = "https://api.anthropic.com/v1/messages"
		case "gemini":
			cfg.Endpoint = "https://generativelanguage.googleapis.com/v1beta/openai/chat/completions"
		case "ollama":
			cfg.Endpoint = "http://localhost:11434/v1/chat/completions"
		default: // "openai"
			cfg.Endpoint = "https://api.openai.com/v1/chat/completions"
		}
	} else if provider != "anthropic" {
		// Normalize base URLs missing /chat/completions
		trimmed := strings.TrimRight(cfg.Endpoint, "/")
		if !strings.HasSuffix(trimmed, "/chat/completions") && !strings.HasSuffix(trimmed, "/completions") {
			cfg.Endpoint = trimmed + "/chat/completions"
		}
	}

	if cfg.Model == "" {
		switch provider {
		case "anthropic":
			cfg.Model = "claude-3-5-haiku-latest"
		case "gemini":
			cfg.Model = "gemini-2.0-flash"
		case "ollama":
			cfg.Model = "llama3.2"
		default:
			cfg.Model = "gpt-4o-mini"
		}
	}

	return &LLMReconciler{
		cfg: cfg,
		httpClient: &http.Client{
			Timeout: cfg.Timeout,
		},
	}
}

// Config returns the reconciler's configured options.
func (l *LLMReconciler) Config() LLMConfig {
	return l.cfg
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

type anthropicMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type anthropicRequest struct {
	Model       string             `json:"model"`
	MaxTokens   int                `json:"max_tokens"`
	System      string             `json:"system,omitempty"`
	Messages    []anthropicMessage `json:"messages"`
	Temperature float32            `json:"temperature"`
}

type anthropicContentBlock struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type anthropicResponse struct {
	Content []anthropicContentBlock `json:"content"`
	Error   *struct {
		Type    string `json:"type"`
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

	var rawContent string
	var err error

	if l.cfg.Provider == "anthropic" {
		rawContent, err = l.callAnthropic(ctx, systemPrompt, userPrompt)
	} else {
		rawContent, err = l.callOpenAI(ctx, systemPrompt, userPrompt)
	}
	if err != nil {
		return nil, err
	}

	cleanJSON := extractJSON(rawContent)
	var output llmOutput
	if err := json.Unmarshal([]byte(cleanJSON), &output); err != nil {
		return nil, fmt.Errorf("unmarshal operations json: %w (raw: %s)", err, rawContent)
	}

	return output.Operations, nil
}

func (l *LLMReconciler) callOpenAI(ctx context.Context, systemPrompt, userPrompt string) (string, error) {
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
		return "", fmt.Errorf("marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", l.cfg.Endpoint, bytes.NewReader(payload))
	if err != nil {
		return "", fmt.Errorf("create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if l.cfg.APIKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+l.cfg.APIKey)
	}

	resp, err := l.httpClient.Do(httpReq)
	if err != nil {
		return "", fmt.Errorf("call llm api: %w", err)
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read response body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("llm api error (status %d): %s", resp.StatusCode, string(bodyBytes))
	}

	var openAIResp openAIResponse
	if err := json.Unmarshal(bodyBytes, &openAIResp); err != nil {
		return "", fmt.Errorf("unmarshal response: %w", err)
	}

	if openAIResp.Error != nil {
		return "", fmt.Errorf("llm api returned error: %s", openAIResp.Error.Message)
	}

	if len(openAIResp.Choices) == 0 {
		return "", fmt.Errorf("no completion choices returned")
	}

	return openAIResp.Choices[0].Message.Content, nil
}

func (l *LLMReconciler) callAnthropic(ctx context.Context, systemPrompt, userPrompt string) (string, error) {
	reqBody := anthropicRequest{
		Model:       l.cfg.Model,
		MaxTokens:   4096,
		System:      systemPrompt,
		Messages:    []anthropicMessage{{Role: "user", Content: userPrompt}},
		Temperature: 0.1,
	}

	payload, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("marshal anthropic request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", l.cfg.Endpoint, bytes.NewReader(payload))
	if err != nil {
		return "", fmt.Errorf("create anthropic request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("anthropic-version", "2023-06-01")
	if l.cfg.APIKey != "" {
		httpReq.Header.Set("x-api-key", l.cfg.APIKey)
	}

	resp, err := l.httpClient.Do(httpReq)
	if err != nil {
		return "", fmt.Errorf("call anthropic api: %w", err)
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read anthropic response body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("anthropic api error (status %d): %s", resp.StatusCode, string(bodyBytes))
	}

	var anthropicResp anthropicResponse
	if err := json.Unmarshal(bodyBytes, &anthropicResp); err != nil {
		return "", fmt.Errorf("unmarshal anthropic response: %w", err)
	}

	if anthropicResp.Error != nil {
		return "", fmt.Errorf("anthropic api returned error: %s", anthropicResp.Error.Message)
	}

	var sb strings.Builder
	for _, block := range anthropicResp.Content {
		if block.Type == "text" {
			sb.WriteString(block.Text)
		}
	}

	if sb.Len() == 0 {
		return "", fmt.Errorf("anthropic returned empty content")
	}

	return sb.String(), nil
}

// extractJSON removes markdown code fences and surrounding conversational text,
// isolating the innermost valid JSON object string.
func extractJSON(raw string) string {
	s := strings.TrimSpace(raw)
	if s == "" {
		return ""
	}

	// 1. Strip markdown fences if present
	if strings.HasPrefix(s, "```") {
		lines := strings.Split(s, "\n")
		if len(lines) >= 2 {
			lines = lines[1:]
			if len(lines) > 0 && strings.HasPrefix(strings.TrimSpace(lines[len(lines)-1]), "```") {
				lines = lines[:len(lines)-1]
			}
			s = strings.TrimSpace(strings.Join(lines, "\n"))
		}
	}

	// 2. Locate boundaries of the outermost JSON object
	firstBrace := strings.Index(s, "{")
	lastBrace := strings.LastIndex(s, "}")
	if firstBrace != -1 && lastBrace != -1 && lastBrace > firstBrace {
		return s[firstBrace : lastBrace+1]
	}

	return s
}
