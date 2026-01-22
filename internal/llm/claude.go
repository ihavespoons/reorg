package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
)

// ClaudeClient implements the Client interface using Claude API
type ClaudeClient struct {
	client anthropic.Client
	model  string
}

// NewClaudeClient creates a new Claude client
func NewClaudeClient(cfg Config) (*ClaudeClient, error) {
	model := cfg.Model
	if model == "" {
		model = "claude-sonnet-4-20250514"
	}

	apiKey, err := resolveClaudeCredentials(cfg)
	if err != nil {
		return nil, err
	}

	client := anthropic.NewClient(option.WithAPIKey(apiKey))

	return &ClaudeClient{
		client: client,
		model:  model,
	}, nil
}

// resolveClaudeCredentials finds API key from various sources
func resolveClaudeCredentials(cfg Config) (string, error) {
	// 1. Explicit API key in config
	if cfg.APIKey != "" {
		return cfg.APIKey, nil
	}

	// 2. Environment variables
	if key := os.Getenv("ANTHROPIC_API_KEY"); key != "" {
		return key, nil
	}
	if key := os.Getenv("CLAUDE_API_KEY"); key != "" {
		return key, nil
	}

	// 3. Credentials file (~/.config/anthropic/credentials)
	if key, err := readCredentialsFile(); err == nil && key != "" {
		return key, nil
	}

	return "", fmt.Errorf(`no Claude API key found

To use Claude, provide an API key via one of these methods:

1. Environment variable:
   export ANTHROPIC_API_KEY="sk-ant-..."

2. Config file (~/.reorg/config.yaml):
   llm:
     api_key: sk-ant-...

3. Credentials file (~/.config/anthropic/credentials):
   Create the file with your API key

Get your API key from https://console.anthropic.com/settings/keys

Note: If you have Claude Code installed and logged in, reorg will
automatically use it as a fallback when no API key is configured`)
}

// readCredentialsFile reads API key from credentials file
func readCredentialsFile() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}

	credPaths := []string{
		filepath.Join(home, ".config", "anthropic", "credentials"),
		filepath.Join(home, ".anthropic", "credentials"),
	}

	for _, path := range credPaths {
		if data, err := os.ReadFile(path); err == nil {
			// Try JSON format first
			var creds struct {
				APIKey string `json:"api_key"`
			}
			if json.Unmarshal(data, &creds) == nil && creds.APIKey != "" {
				return creds.APIKey, nil
			}
			// Fall back to plain text
			key := strings.TrimSpace(string(data))
			if key != "" {
				return key, nil
			}
		}
	}

	return "", fmt.Errorf("no credentials file found")
}

// Provider returns the provider type
func (c *ClaudeClient) Provider() Provider {
	return ProviderClaude
}

// Categorize analyzes text and returns categorization
func (c *ClaudeClient) Categorize(ctx context.Context, content string) (*CategorizeResult, error) {
	prompt := fmt.Sprintf(`Analyze the following content and categorize it for a personal organization system.

Determine:
1. Which area it belongs to: "work", "personal", or "life-admin"
   - "work" = professional tasks, job-related, clients, colleagues, meetings
   - "personal" = hobbies, personal projects, relationships, health, learning
   - "life-admin" = bills, appointments, paperwork, household tasks, errands
2. Suggest a project name if this is part of a larger effort
3. Extract relevant tags
4. Provide a brief summary
5. Determine if it contains actionable items

Content:
%s

Respond with valid JSON only, no markdown formatting:
{
  "area": "work|personal|life-admin",
  "area_confidence": 0.0-1.0,
  "project_suggestion": "suggested project name or empty",
  "tags": ["tag1", "tag2"],
  "summary": "brief summary",
  "is_actionable": true|false
}`, content)

	response, err := c.client.Messages.New(ctx, anthropic.MessageNewParams{
		Model:     anthropic.Model(c.model),
		MaxTokens: 1024,
		Messages: []anthropic.MessageParam{
			anthropic.NewUserMessage(anthropic.NewTextBlock(prompt)),
		},
	})
	if err != nil {
		return nil, fmt.Errorf("claude API error: %w", err)
	}

	// Extract text from response
	var responseText string
	for _, block := range response.Content {
		if block.Type == "text" {
			responseText = block.Text
			break
		}
	}

	if responseText == "" {
		return nil, fmt.Errorf("empty response from Claude")
	}

	var result CategorizeResult
	if err := json.Unmarshal([]byte(responseText), &result); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w (response: %s)", err, responseText)
	}

	return &result, nil
}

// ExtractTasks parses content and extracts actionable tasks
func (c *ClaudeClient) ExtractTasks(ctx context.Context, content string) ([]ExtractedTask, error) {
	prompt := fmt.Sprintf(`Extract actionable tasks from the following content.

For each task, determine:
1. A clear, concise title (action-oriented, starts with verb)
2. Any additional description/context
3. Priority if mentioned or implied (low, medium, high, urgent)
4. Due date if mentioned (format: YYYY-MM-DD)
5. Relevant tags

Content:
%s

Respond with valid JSON only, no markdown formatting:
{
  "tasks": [
    {
      "title": "task title",
      "description": "additional context",
      "priority": "medium",
      "due_date": "2025-01-25",
      "tags": ["tag1"]
    }
  ]
}

If no actionable tasks are found, return: {"tasks": []}`, content)

	response, err := c.client.Messages.New(ctx, anthropic.MessageNewParams{
		Model:     anthropic.Model(c.model),
		MaxTokens: 2048,
		Messages: []anthropic.MessageParam{
			anthropic.NewUserMessage(anthropic.NewTextBlock(prompt)),
		},
	})
	if err != nil {
		return nil, fmt.Errorf("claude API error: %w", err)
	}

	// Extract text from response
	var responseText string
	for _, block := range response.Content {
		if block.Type == "text" {
			responseText = block.Text
			break
		}
	}

	if responseText == "" {
		return nil, fmt.Errorf("empty response from Claude")
	}

	var result struct {
		Tasks []ExtractedTask `json:"tasks"`
	}
	if err := json.Unmarshal([]byte(responseText), &result); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w (response: %s)", err, responseText)
	}

	return result.Tasks, nil
}

// Chat sends a message and returns the response
func (c *ClaudeClient) Chat(ctx context.Context, message string) (string, error) {
	response, err := c.client.Messages.New(ctx, anthropic.MessageNewParams{
		Model:     anthropic.Model(c.model),
		MaxTokens: 4096,
		System: []anthropic.TextBlockParam{
			{Text: "You are a helpful personal organization assistant. You help users manage their tasks, projects, and time effectively. Be concise and action-oriented in your responses."},
		},
		Messages: []anthropic.MessageParam{
			anthropic.NewUserMessage(anthropic.NewTextBlock(message)),
		},
	})
	if err != nil {
		return "", fmt.Errorf("claude API error: %w", err)
	}

	// Extract text from response
	for _, block := range response.Content {
		if block.Type == "text" {
			return block.Text, nil
		}
	}

	return "", fmt.Errorf("empty response from Claude")
}
