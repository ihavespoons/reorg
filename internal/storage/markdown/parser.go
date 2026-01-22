package markdown

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/adrg/frontmatter"
	"gopkg.in/yaml.v3"

	"github.com/beng/reorg/internal/domain"
)

// Parser handles reading and writing markdown files with YAML frontmatter
type Parser struct{}

// NewParser creates a new Parser instance
func NewParser() *Parser {
	return &Parser{}
}

// ParseArea reads a markdown file and parses it into an Area
func (p *Parser) ParseArea(r io.Reader) (*domain.Area, error) {
	var area domain.Area
	content, err := frontmatter.Parse(r, &area)
	if err != nil {
		return nil, fmt.Errorf("failed to parse area frontmatter: %w", err)
	}
	area.Content = strings.TrimSpace(string(content))
	return &area, nil
}

// ParseAreaFromFile reads a file and parses it into an Area
func (p *Parser) ParseAreaFromFile(path string) (*domain.Area, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("failed to open area file: %w", err)
	}
	defer f.Close()
	return p.ParseArea(f)
}

// ParseProject reads a markdown file and parses it into a Project
func (p *Parser) ParseProject(r io.Reader) (*domain.Project, error) {
	var project domain.Project
	content, err := frontmatter.Parse(r, &project)
	if err != nil {
		return nil, fmt.Errorf("failed to parse project frontmatter: %w", err)
	}
	project.Content = strings.TrimSpace(string(content))
	return &project, nil
}

// ParseProjectFromFile reads a file and parses it into a Project
func (p *Parser) ParseProjectFromFile(path string) (*domain.Project, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("failed to open project file: %w", err)
	}
	defer f.Close()
	return p.ParseProject(f)
}

// ParseTask reads a markdown file and parses it into a Task
func (p *Parser) ParseTask(r io.Reader) (*domain.Task, error) {
	var task domain.Task
	content, err := frontmatter.Parse(r, &task)
	if err != nil {
		return nil, fmt.Errorf("failed to parse task frontmatter: %w", err)
	}
	task.Content = strings.TrimSpace(string(content))
	return &task, nil
}

// ParseTaskFromFile reads a file and parses it into a Task
func (p *Parser) ParseTaskFromFile(path string) (*domain.Task, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("failed to open task file: %w", err)
	}
	defer f.Close()
	return p.ParseTask(f)
}

// marshalFrontmatter creates the YAML frontmatter block
func marshalFrontmatter(v interface{}) ([]byte, error) {
	yamlData, err := yaml.Marshal(v)
	if err != nil {
		return nil, err
	}

	var buf bytes.Buffer
	buf.WriteString("---\n")
	buf.Write(yamlData)
	buf.WriteString("---\n")

	return buf.Bytes(), nil
}
