package git

import (
	"fmt"
	"time"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"
)

// Client handles git operations for the reorg data directory
type Client struct {
	repo    *git.Repository
	rootDir string
	enabled bool
}

// NewClient creates a new git client for the given directory
func NewClient(rootDir string) (*Client, error) {
	repo, err := git.PlainOpen(rootDir)
	if err != nil {
		// Not a git repo - that's ok, git is optional
		return &Client{
			rootDir: rootDir,
			enabled: false,
		}, nil
	}

	return &Client{
		repo:    repo,
		rootDir: rootDir,
		enabled: true,
	}, nil
}

// IsEnabled returns true if git is enabled for this directory
func (c *Client) IsEnabled() bool {
	return c.enabled
}

// Init initializes a new git repository in the directory
func (c *Client) Init() error {
	if c.enabled {
		return nil // Already initialized
	}

	repo, err := git.PlainInit(c.rootDir, false)
	if err != nil {
		return fmt.Errorf("failed to init git repo: %w", err)
	}

	c.repo = repo
	c.enabled = true
	return nil
}

// Add stages a file for commit
func (c *Client) Add(path string) error {
	if !c.enabled {
		return nil
	}

	worktree, err := c.repo.Worktree()
	if err != nil {
		return fmt.Errorf("failed to get worktree: %w", err)
	}

	_, err = worktree.Add(path)
	if err != nil {
		return fmt.Errorf("failed to add file: %w", err)
	}

	return nil
}

// AddAll stages all changes for commit
func (c *Client) AddAll() error {
	if !c.enabled {
		return nil
	}

	worktree, err := c.repo.Worktree()
	if err != nil {
		return fmt.Errorf("failed to get worktree: %w", err)
	}

	err = worktree.AddWithOptions(&git.AddOptions{
		All: true,
	})
	if err != nil {
		return fmt.Errorf("failed to add all: %w", err)
	}

	return nil
}

// Commit creates a commit with the given message
func (c *Client) Commit(message string) error {
	if !c.enabled {
		return nil
	}

	worktree, err := c.repo.Worktree()
	if err != nil {
		return fmt.Errorf("failed to get worktree: %w", err)
	}

	// Check if there are changes to commit
	status, err := worktree.Status()
	if err != nil {
		return fmt.Errorf("failed to get status: %w", err)
	}

	if status.IsClean() {
		return nil // Nothing to commit
	}

	_, err = worktree.Commit(message, &git.CommitOptions{
		Author: &object.Signature{
			Name:  "reorg",
			Email: "reorg@localhost",
			When:  time.Now(),
		},
	})
	if err != nil {
		return fmt.Errorf("failed to commit: %w", err)
	}

	return nil
}

// AutoCommit adds all changes and commits with a prefixed message
func (c *Client) AutoCommit(action string) error {
	if !c.enabled {
		return nil
	}

	if err := c.AddAll(); err != nil {
		return err
	}

	message := fmt.Sprintf("reorg: %s", action)
	return c.Commit(message)
}

// Status returns the current status of the repository
func (c *Client) Status() (string, error) {
	if !c.enabled {
		return "git not enabled", nil
	}

	worktree, err := c.repo.Worktree()
	if err != nil {
		return "", fmt.Errorf("failed to get worktree: %w", err)
	}

	status, err := worktree.Status()
	if err != nil {
		return "", fmt.Errorf("failed to get status: %w", err)
	}

	if status.IsClean() {
		return "clean", nil
	}

	return status.String(), nil
}
