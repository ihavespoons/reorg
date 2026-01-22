package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// OllamaClient implements the Client interface using Ollama
type OllamaClient struct {
	baseURL string
	model   string
	client  *http.Client
}

// NewOllamaClient creates a new Ollama client
func NewOllamaClient(baseURL, model string) (*OllamaClient, error) {
	if baseURL == "" {
		baseURL = "http://localhost:11434"
	}

	if model == "" {
		model = "llama3.2"
	}

	return &OllamaClient{
		baseURL: strings.TrimSuffix(baseURL, "/"),
		model:   model,
		client:  &http.Client{},
	}, nil
}

// Provider returns the provider type
func (c *OllamaClient) Provider() Provider {
	return ProviderOllama
}

type ollamaRequest struct {
	Model  string `json:"model"`
	Prompt string `json:"prompt"`
	Stream bool   `json:"stream"`
}

type ollamaResponse struct {
	Response string `json:"response"`
	Done     bool   `json:"done"`
}

func (c *OllamaClient) generate(ctx context.Context, prompt string) (string, error) {
	reqBody := ollamaRequest{
		Model:  c.model,
		Prompt: prompt,
		Stream: false,
	}

	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", c.baseURL+"/api/generate", bytes.NewBuffer(jsonBody))
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("ollama request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("ollama error (status %d): %s", resp.StatusCode, string(body))
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response: %w", err)
	}

	var result ollamaResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return "", fmt.Errorf("failed to parse response: %w", err)
	}

	return result.Response, nil
}

// Categorize analyzes text and returns categorization
func (c *OllamaClient) Categorize(ctx context.Context, content string) (*CategorizeResult, error) {
	prompt := fmt.Sprintf(`Analyze the following content and categorize it.

Areas: "work", "personal", or "life-admin"
- work = professional tasks, job-related
- personal = hobbies, personal projects
- life-admin = bills, appointments, errands

Content: %s

Respond with JSON only:
{"area": "work", "area_confidence": 0.8, "project_suggestion": "", "tags": [], "summary": "", "is_actionable": false}`, content)

	response, err := c.generate(ctx, prompt)
	if err != nil {
		return nil, err
	}

	response = extractJSON(response)

	var result CategorizeResult
	if err := json.Unmarshal([]byte(response), &result); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return &result, nil
}

// ExtractTasks parses content and extracts actionable tasks
func (c *OllamaClient) ExtractTasks(ctx context.Context, content string) ([]ExtractedTask, error) {
	prompt := fmt.Sprintf(`Extract tasks from this content. Return JSON only.

Content: %s

Format: {"tasks": [{"title": "", "description": "", "priority": "medium", "due_date": "", "tags": []}]}`, content)

	response, err := c.generate(ctx, prompt)
	if err != nil {
		return nil, err
	}

	response = extractJSON(response)

	var result struct {
		Tasks []ExtractedTask `json:"tasks"`
	}
	if err := json.Unmarshal([]byte(response), &result); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return result.Tasks, nil
}

// Chat sends a message and returns the response
func (c *OllamaClient) Chat(ctx context.Context, message string) (string, error) {
	prompt := fmt.Sprintf("You are a helpful personal organization assistant. Be concise.\n\nUser: %s\n\nAssistant:", message)
	return c.generate(ctx, prompt)
}

// extractJSON tries to extract JSON from a response that might contain extra text
func extractJSON(s string) string {
	// Find first { and last }
	start := strings.Index(s, "{")
	end := strings.LastIndex(s, "}")
	if start >= 0 && end > start {
		return s[start : end+1]
	}
	return s
}
