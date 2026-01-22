package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// ClaudeCodeClient implements the Client interface by shelling out to Claude Code CLI
type ClaudeCodeClient struct {
	model string
}

// NewClaudeCodeClient creates a new client that uses Claude Code CLI
func NewClaudeCodeClient(model string) (*ClaudeCodeClient, error) {
	// Check if claude CLI is available
	if _, err := exec.LookPath("claude"); err != nil {
		return nil, fmt.Errorf("claude CLI not found: %w", err)
	}

	return &ClaudeCodeClient{
		model: model,
	}, nil
}

// IsClaudeCodeAvailable checks if Claude Code CLI is installed and authenticated
func IsClaudeCodeAvailable() bool {
	// Check if claude command exists
	if _, err := exec.LookPath("claude"); err != nil {
		return false
	}

	// Quick check - try to run a simple prompt
	// Use timeout to avoid hanging
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "claude", "-p", "--output-format", "text", "--tools", "", "Say OK")
	output, err := cmd.Output()
	if err != nil {
		return false
	}

	return strings.Contains(strings.ToLower(string(output)), "ok")
}

// Provider returns the provider type
func (c *ClaudeCodeClient) Provider() Provider {
	return ProviderClaudeCode
}

// runPrompt executes a prompt via Claude Code CLI and returns the response
func (c *ClaudeCodeClient) runPrompt(ctx context.Context, prompt string) (string, error) {
	args := []string{
		"-p",                  // Print mode (non-interactive)
		"--output-format", "text",
		"--tools", "",        // Disable all tools
	}

	if c.model != "" {
		args = append(args, "--model", c.model)
	}

	cmd := exec.CommandContext(ctx, "claude", args...)

	// Pass prompt via stdin to handle multiline and special characters
	cmd.Stdin = strings.NewReader(prompt)

	output, err := cmd.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return "", fmt.Errorf("claude CLI error: %s", string(exitErr.Stderr))
		}
		return "", fmt.Errorf("failed to execute claude CLI: %w", err)
	}

	return strings.TrimSpace(string(output)), nil
}

// Categorize analyzes text and returns categorization
func (c *ClaudeCodeClient) Categorize(ctx context.Context, content string) (*CategorizeResult, error) {
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

	responseText, err := c.runPrompt(ctx, prompt)
	if err != nil {
		return nil, err
	}

	// Clean up response - remove any markdown code blocks if present
	responseText = cleanJSONResponse(responseText)

	var result CategorizeResult
	if err := json.Unmarshal([]byte(responseText), &result); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w (response: %s)", err, responseText)
	}

	return &result, nil
}

// ExtractTasks parses content and extracts actionable tasks
func (c *ClaudeCodeClient) ExtractTasks(ctx context.Context, content string) ([]ExtractedTask, error) {
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

	responseText, err := c.runPrompt(ctx, prompt)
	if err != nil {
		return nil, err
	}

	// Clean up response
	responseText = cleanJSONResponse(responseText)

	var result struct {
		Tasks []ExtractedTask `json:"tasks"`
	}
	if err := json.Unmarshal([]byte(responseText), &result); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w (response: %s)", err, responseText)
	}

	return result.Tasks, nil
}

// Chat sends a message and returns the response
func (c *ClaudeCodeClient) Chat(ctx context.Context, message string) (string, error) {
	return c.runPrompt(ctx, message)
}

// cleanJSONResponse removes markdown code blocks from a response
func cleanJSONResponse(s string) string {
	s = strings.TrimSpace(s)

	// Remove ```json and ``` if present
	if strings.HasPrefix(s, "```json") {
		s = strings.TrimPrefix(s, "```json")
		s = strings.TrimSuffix(s, "```")
		s = strings.TrimSpace(s)
	} else if strings.HasPrefix(s, "```") {
		s = strings.TrimPrefix(s, "```")
		s = strings.TrimSuffix(s, "```")
		s = strings.TrimSpace(s)
	}

	return s
}
