package markdown

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/ihavespoons/reorg/internal/domain"
)

// Writer handles writing domain objects to markdown files
type Writer struct{}

// NewWriter creates a new Writer instance
func NewWriter() *Writer {
	return &Writer{}
}

// WriteArea writes an Area to a writer as markdown with YAML frontmatter
func (w *Writer) WriteArea(out io.Writer, area *domain.Area) error {
	fm, err := marshalFrontmatter(area)
	if err != nil {
		return fmt.Errorf("failed to marshal area frontmatter: %w", err)
	}

	if _, err := out.Write(fm); err != nil {
		return fmt.Errorf("failed to write frontmatter: %w", err)
	}

	if area.Content != "" {
		if _, err := out.Write([]byte("\n" + area.Content + "\n")); err != nil {
			return fmt.Errorf("failed to write content: %w", err)
		}
	} else {
		// Write default content structure
		defaultContent := fmt.Sprintf("\n# %s\n\nDescription goes here.\n", area.Title)
		if _, err := out.Write([]byte(defaultContent)); err != nil {
			return fmt.Errorf("failed to write default content: %w", err)
		}
	}

	return nil
}

// WriteAreaToFile writes an Area to a file
func (w *Writer) WriteAreaToFile(path string, area *domain.Area) error {
	// Ensure directory exists
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("failed to create file: %w", err)
	}
	defer func() { _ = f.Close() }()

	return w.WriteArea(f, area)
}

// WriteProject writes a Project to a writer as markdown with YAML frontmatter
func (w *Writer) WriteProject(out io.Writer, project *domain.Project) error {
	fm, err := marshalFrontmatter(project)
	if err != nil {
		return fmt.Errorf("failed to marshal project frontmatter: %w", err)
	}

	if _, err := out.Write(fm); err != nil {
		return fmt.Errorf("failed to write frontmatter: %w", err)
	}

	if project.Content != "" {
		if _, err := out.Write([]byte("\n" + project.Content + "\n")); err != nil {
			return fmt.Errorf("failed to write content: %w", err)
		}
	} else {
		// Write default content structure
		defaultContent := fmt.Sprintf("\n# %s\n\nProject description and notes.\n\n## Objectives\n\n- \n\n## Notes\n\n", project.Title)
		if _, err := out.Write([]byte(defaultContent)); err != nil {
			return fmt.Errorf("failed to write default content: %w", err)
		}
	}

	return nil
}

// WriteProjectToFile writes a Project to a file
func (w *Writer) WriteProjectToFile(path string, project *domain.Project) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("failed to create file: %w", err)
	}
	defer func() { _ = f.Close() }()

	return w.WriteProject(f, project)
}

// WriteTask writes a Task to a writer as markdown with YAML frontmatter
func (w *Writer) WriteTask(out io.Writer, task *domain.Task) error {
	fm, err := marshalFrontmatter(task)
	if err != nil {
		return fmt.Errorf("failed to marshal task frontmatter: %w", err)
	}

	if _, err := out.Write(fm); err != nil {
		return fmt.Errorf("failed to write frontmatter: %w", err)
	}

	if task.Content != "" {
		if _, err := out.Write([]byte("\n" + task.Content + "\n")); err != nil {
			return fmt.Errorf("failed to write content: %w", err)
		}
	} else {
		// Write default content structure
		defaultContent := fmt.Sprintf("\n# %s\n\nTask description.\n\n## Checklist\n\n- [ ] \n\n## Notes\n\n", task.Title)
		if _, err := out.Write([]byte(defaultContent)); err != nil {
			return fmt.Errorf("failed to write default content: %w", err)
		}
	}

	return nil
}

// WriteTaskToFile writes a Task to a file
func (w *Writer) WriteTaskToFile(path string, task *domain.Task) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("failed to create file: %w", err)
	}
	defer func() { _ = f.Close() }()

	return w.WriteTask(f, task)
}

// MarshalArea returns the markdown representation of an Area
func (w *Writer) MarshalArea(area *domain.Area) ([]byte, error) {
	var buf bytes.Buffer
	if err := w.WriteArea(&buf, area); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// MarshalProject returns the markdown representation of a Project
func (w *Writer) MarshalProject(project *domain.Project) ([]byte, error) {
	var buf bytes.Buffer
	if err := w.WriteProject(&buf, project); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// MarshalTask returns the markdown representation of a Task
func (w *Writer) MarshalTask(task *domain.Task) ([]byte, error) {
	var buf bytes.Buffer
	if err := w.WriteTask(&buf, task); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

