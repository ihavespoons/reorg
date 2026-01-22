package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
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

// NewClaudeClient creates a new Claude client with support for multiple auth methods
func NewClaudeClient(cfg Config) (*ClaudeClient, error) {
	model := cfg.Model
	if model == "" {
		model = "claude-sonnet-4-20250514"
	}

	// Try to get credentials in order of priority
	apiKey, authMethod, err := resolveClaudeCredentials(cfg)
	if err != nil {
		return nil, err
	}

	var opts []option.RequestOption

	switch authMethod {
	case AuthMethodOAuth:
		// OAuth uses Authorization: Bearer header
		opts = append(opts, option.WithHeader("Authorization", "Bearer "+apiKey))
	default:
		// Standard API key authentication
		opts = append(opts, option.WithAPIKey(apiKey))
	}

	client := anthropic.NewClient(opts...)

	return &ClaudeClient{
		client: client,
		model:  model,
	}, nil
}

// resolveClaudeCredentials finds credentials from various sources
func resolveClaudeCredentials(cfg Config) (string, AuthMethod, error) {
	// 1. Explicit API key in config
	if cfg.APIKey != "" {
		return cfg.APIKey, AuthMethodAPIKey, nil
	}

	// 2. Explicit OAuth token in config
	if cfg.OAuthToken != "" {
		return cfg.OAuthToken, AuthMethodOAuth, nil
	}

	// 3. Environment variables
	if key := os.Getenv("ANTHROPIC_API_KEY"); key != "" {
		return key, AuthMethodAPIKey, nil
	}
	if key := os.Getenv("CLAUDE_API_KEY"); key != "" {
		return key, AuthMethodAPIKey, nil
	}

	// 4. Claude Max OAuth token from Claude Code or Claude Desktop
	if token, err := findClaudeOAuthToken(); err == nil && token != "" {
		return token, AuthMethodOAuth, nil
	}

	// 5. Credentials file (~/.config/anthropic/credentials)
	if key, err := readCredentialsFile(); err == nil && key != "" {
		return key, AuthMethodAPIKey, nil
	}

	return "", "", fmt.Errorf(`no Claude credentials found

To use Claude, provide credentials via one of these methods:

1. API Key (works with Claude Max or API plans):
   - Set ANTHROPIC_API_KEY environment variable
   - Or add to ~/.reorg/config.yaml:
     llm:
       api_key: your-key-here

   Get your API key from: https://console.anthropic.com/settings/keys

2. OAuth (Claude Max subscription):
   - Log in with Claude Code CLI: claude login
   - Your OAuth session will be shared automatically

3. Credentials file:
   - Create ~/.config/anthropic/credentials with your API key`)
}

// findClaudeOAuthToken looks for OAuth tokens from Claude Code or Claude Desktop
func findClaudeOAuthToken() (string, error) {
	// On macOS, check the keychain first (where Claude Code stores credentials)
	if token, err := findKeychainCredentials(); err == nil && token != "" {
		return token, nil
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}

	// Check Claude Code OAuth token locations (fallback for file-based storage)
	tokenPaths := []string{
		filepath.Join(home, ".claude", "oauth_token"),
		filepath.Join(home, ".claude", "credentials.json"),
		filepath.Join(home, ".config", "claude", "oauth_token"),
		filepath.Join(home, "Library", "Application Support", "Claude", "oauth_token"),
	}

	for _, path := range tokenPaths {
		if data, err := os.ReadFile(path); err == nil {
			token := strings.TrimSpace(string(data))
			if token != "" {
				// Handle JSON format if needed
				if strings.HasPrefix(token, "{") {
					var creds struct {
						AccessToken string `json:"access_token"`
						Token       string `json:"token"`
						APIKey      string `json:"api_key"`
					}
					if json.Unmarshal(data, &creds) == nil {
						if creds.AccessToken != "" {
							return creds.AccessToken, nil
						}
						if creds.Token != "" {
							return creds.Token, nil
						}
						if creds.APIKey != "" {
							return creds.APIKey, nil
						}
					}
				}
				return token, nil
			}
		}
	}

	return "", fmt.Errorf("no OAuth token found")
}

// findKeychainCredentials reads Claude credentials from macOS keychain
func findKeychainCredentials() (string, error) {
	// Get current username for keychain lookup
	currentUser := os.Getenv("USER")
	if currentUser == "" {
		currentUser = os.Getenv("LOGNAME")
	}

	// Try Claude Code credentials with username
	if currentUser != "" {
		token, err := readFromKeychainWithAccount("Claude Code-credentials", currentUser)
		if err == nil && token != "" {
			return token, nil
		}
	}

	// Try different keychain service names used by Claude Code
	keychainServices := []string{
		"Claude Code-credentials",
		"claude-code-credentials",
		"anthropic",
		"claude",
	}

	for _, service := range keychainServices {
		token, err := readFromKeychain(service)
		if err == nil && token != "" {
			return token, nil
		}
	}

	return "", fmt.Errorf("no keychain credentials found")
}

// readFromKeychainWithAccount reads a password from macOS keychain with specific account
func readFromKeychainWithAccount(service, account string) (string, error) {
	cmd := exec.Command("security", "find-generic-password", "-s", service, "-a", account, "-w")
	output, err := cmd.Output()
	if err != nil {
		return "", err
	}

	return parseKeychainOutput(string(output))
}

// readFromKeychain reads a password from macOS keychain using security command
func readFromKeychain(service string) (string, error) {
	cmd := exec.Command("security", "find-generic-password", "-s", service, "-w")
	output, err := cmd.Output()
	if err != nil {
		return "", err
	}

	return parseKeychainOutput(string(output))
}

// parseKeychainOutput extracts the OAuth token from keychain data
func parseKeychainOutput(output string) (string, error) {
	token := strings.TrimSpace(output)
	if token == "" {
		return "", fmt.Errorf("empty token")
	}

	// The keychain stores JSON with Claude OAuth credentials
	if strings.HasPrefix(token, "{") {
		// Try Claude Code format: {"claudeAiOauth":{"accessToken":"..."}}
		var claudeCodeCreds struct {
			ClaudeAiOauth struct {
				AccessToken  string `json:"accessToken"`
				RefreshToken string `json:"refreshToken"`
				ExpiresAt    string `json:"expiresAt"`
			} `json:"claudeAiOauth"`
		}
		if json.Unmarshal([]byte(token), &claudeCodeCreds) == nil && claudeCodeCreds.ClaudeAiOauth.AccessToken != "" {
			return claudeCodeCreds.ClaudeAiOauth.AccessToken, nil
		}

		// Try simple format: {"accessToken":"..."}
		var simpleCreds struct {
			AccessToken  string `json:"accessToken"`
			RefreshToken string `json:"refreshToken"`
			ExpiresAt    string `json:"expiresAt"`
		}
		if json.Unmarshal([]byte(token), &simpleCreds) == nil && simpleCreds.AccessToken != "" {
			return simpleCreds.AccessToken, nil
		}
	}

	return token, nil
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
