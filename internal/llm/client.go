package llm

import (
	"context"
)

// Provider identifies an LLM provider
type Provider string

const (
	ProviderClaude Provider = "claude"
	ProviderOllama Provider = "ollama"
)

// Client defines the interface for LLM operations
type Client interface {
	// Categorize analyzes text and returns categorization
	Categorize(ctx context.Context, content string) (*CategorizeResult, error)

	// ExtractTasks parses content and extracts actionable tasks
	ExtractTasks(ctx context.Context, content string) ([]ExtractedTask, error)

	// Chat sends a message and returns the response
	Chat(ctx context.Context, message string) (string, error)

	// Provider returns the provider type
	Provider() Provider
}

// CategorizeResult contains the categorization of content
type CategorizeResult struct {
	// Area suggests which area this content belongs to (work, personal, life-admin)
	Area string `json:"area"`

	// AreaConfidence is a score from 0-1 indicating confidence
	AreaConfidence float64 `json:"area_confidence"`

	// ProjectSuggestion suggests a project name if applicable
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

// AuthMethod specifies how to authenticate with the LLM provider
type AuthMethod string

const (
	AuthMethodAPIKey AuthMethod = "api_key"
	AuthMethodOAuth  AuthMethod = "oauth"
)

// Config holds LLM configuration
type Config struct {
	Provider   Provider
	AuthMethod AuthMethod
	APIKey     string
	OAuthToken string
	Model      string
	BaseURL    string // For Ollama or custom endpoints
}

// NewClient creates a new LLM client based on configuration
func NewClient(cfg Config) (Client, error) {
	switch cfg.Provider {
	case ProviderClaude:
		return NewClaudeClient(cfg)
	case ProviderOllama:
		return NewOllamaClient(cfg.BaseURL, cfg.Model)
	default:
		return NewClaudeClient(cfg)
	}
}
