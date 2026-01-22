package llm

import (
	"context"
	"os"
)

// Provider identifies an LLM provider
type Provider string

const (
	ProviderClaude     Provider = "claude"
	ProviderClaudeCode Provider = "claude-code"
	ProviderOllama     Provider = "ollama"
)

// Client defines the interface for LLM operations
type Client interface {
	// Categorize analyzes text and returns categorization
	Categorize(ctx context.Context, content string) (*CategorizeResult, error)

	// CategorizeWithContext analyzes text with knowledge of existing projects
	CategorizeWithContext(ctx context.Context, content string, existingProjects []ProjectContext) (*CategorizeResult, error)

	// ExtractTasks parses content and extracts actionable tasks
	ExtractTasks(ctx context.Context, content string) ([]ExtractedTask, error)

	// Chat sends a message and returns the response
	Chat(ctx context.Context, message string) (string, error)

	// Provider returns the provider type
	Provider() Provider
}

// ProjectContext provides context about an existing project for AI matching
type ProjectContext struct {
	ID    string `json:"id"`
	Title string `json:"title"`
	Area  string `json:"area"`
}

// CategorizeResult contains the categorization of content
type CategorizeResult struct {
	// Area suggests which area this content belongs to (work, personal, life-admin)
	Area string `json:"area"`

	// AreaConfidence is a score from 0-1 indicating confidence
	AreaConfidence float64 `json:"area_confidence"`

	// ProjectID is the ID of an existing project if matched (empty if new project)
	ProjectID string `json:"project_id,omitempty"`

	// ProjectSuggestion suggests a project name if applicable (for new projects)
	ProjectSuggestion string `json:"project_suggestion,omitempty"`

	// Tags are suggested tags for the content
	Tags []string `json:"tags,omitempty"`

	// Summary is a brief summary of the content
	Summary string `json:"summary"`

	// IsActionable indicates if the content contains actionable items
	IsActionable bool `json:"is_actionable"`
}

// ExtractedTask represents a task extracted from content
type ExtractedTask struct {
	// Title is the task title
	Title string `json:"title"`

	// Description provides more context
	Description string `json:"description,omitempty"`

	// Priority suggests the task priority
	Priority string `json:"priority,omitempty"`

	// DueDate suggests a due date if mentioned
	DueDate string `json:"due_date,omitempty"`

	// Tags are suggested tags
	Tags []string `json:"tags,omitempty"`
}

// Config holds LLM configuration
type Config struct {
	Provider Provider
	APIKey   string
	Model    string
	BaseURL  string // For Ollama or custom endpoints
}

// NewClient creates a new LLM client based on configuration
func NewClient(cfg Config) (Client, error) {
	switch cfg.Provider {
	case ProviderClaude:
		return NewClaudeClient(cfg)
	case ProviderClaudeCode:
		return NewClaudeCodeClient(cfg.Model)
	case ProviderOllama:
		return NewOllamaClient(cfg.BaseURL, cfg.Model)
	default:
		return NewClaudeClient(cfg)
	}
}

// NewClientWithFallback creates a client, preferring Claude Code CLI when no explicit API key is set
func NewClientWithFallback(cfg Config) (Client, error) {
	// If explicit API key is provided, use the standard Claude API
	if cfg.APIKey != "" || cfg.Provider == ProviderOllama {
		return NewClient(cfg)
	}

	// Check environment variables for API key
	if os.Getenv("ANTHROPIC_API_KEY") != "" || os.Getenv("CLAUDE_API_KEY") != "" {
		return NewClient(cfg)
	}

	// No explicit API key - prefer Claude Code CLI if available
	// (OAuth tokens from keychain can't be used with the public API)
	if cfg.Provider == ProviderClaude || cfg.Provider == "" {
		if codeClient, err := NewClaudeCodeClient(cfg.Model); err == nil {
			return codeClient, nil
		}
	}

	// Fall back to standard client (will show appropriate error for missing credentials)
	return NewClient(cfg)
}
