package cli

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
	"text/tabwriter"

	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"

	"github.com/beng/reorg/internal/domain"
)

var (
	projectAreaFlag     string
	projectPriorityFlag string
	projectTagsFlag     []string
)

var projectCmd = &cobra.Command{
	Use:   "project",
	Short: "Manage projects",
	Long:  `Projects are collections of related tasks within an area.`,
}

var projectListCmd = &cobra.Command{
	Use:   "list",
	Short: "List projects",
	RunE:  runProjectList,
}

var projectCreateCmd = &cobra.Command{
	Use:   "create [name]",
	Short: "Create a new project",
	Args:  cobra.ExactArgs(1),
	RunE:  runProjectCreate,
}

var projectShowCmd = &cobra.Command{
	Use:   "show [project]",
	Short: "Show project details",
	Args:  cobra.ExactArgs(1),
	RunE:  runProjectShow,
}

var projectCompleteCmd = &cobra.Command{
	Use:   "complete [project]",
	Short: "Mark a project as completed",
	Args:  cobra.ExactArgs(1),
	RunE:  runProjectComplete,
}

var projectDeleteCmd = &cobra.Command{
	Use:   "delete [project]",
	Short: "Delete a project",
	Args:  cobra.ExactArgs(1),
	RunE:  runProjectDelete,
}

func init() {
	rootCmd.AddCommand(projectCmd)
	projectCmd.AddCommand(projectListCmd)
	projectCmd.AddCommand(projectCreateCmd)
	projectCmd.AddCommand(projectShowCmd)
	projectCmd.AddCommand(projectCompleteCmd)
	projectCmd.AddCommand(projectDeleteCmd)

	// List flags
	projectListCmd.Flags().StringVarP(&projectAreaFlag, "area", "a", "", "Filter by area")

	// Create flags
	projectCreateCmd.Flags().StringVarP(&projectAreaFlag, "area", "a", "", "Area for the project")
	projectCreateCmd.Flags().StringVarP(&projectPriorityFlag, "priority", "p", "medium", "Priority (low, medium, high, urgent)")
	projectCreateCmd.Flags().StringSliceVarP(&projectTagsFlag, "tags", "t", nil, "Tags for the project")
}

func runProjectList(cmd *cobra.Command, args []string) error {
	ctx := context.Background()

	var projects []*domain.Project
	var err error

	if projectAreaFlag != "" {
		// Get area by slug
		area, err := client.GetAreaBySlug(ctx, projectAreaFlag)
		if err != nil {
			return fmt.Errorf("area not found: %s", projectAreaFlag)
		}
		projects, err = client.ListProjects(ctx, area.ID)
		if err != nil {
			return fmt.Errorf("failed to list projects: %w", err)
		}
	} else {
		projects, err = client.ListAllProjects(ctx)
		if err != nil {
			return fmt.Errorf("failed to list projects: %w", err)
		}
	}

	if len(projects) == 0 {
		fmt.Println("No projects found. Create one with 'reorg project create <name>'")
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	_, _ = fmt.Fprintln(w, "PROJECT\tAREA\tSTATUS\tPRIORITY\tTASKS")
	_, _ = fmt.Fprintln(w, "-------\t----\t------\t--------\t-----")

	for _, p := range projects {
		// Get area name
		area, _ := client.GetArea(ctx, p.AreaID)
		areaName := ""
		if area != nil {
			areaName = area.Title
		}

		// Count tasks
		tasks, _ := client.ListTasks(ctx, p.ID)
		completedTasks := 0
		for _, t := range tasks {
			if t.IsComplete() {
				completedTasks++
			}
		}
		taskStr := fmt.Sprintf("%d/%d", completedTasks, len(tasks))

		_, _ = fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n",
			p.Title,
			areaName,
			p.Status,
			p.Priority,
			taskStr,
		)
	}

	return w.Flush()
}

func runProjectCreate(cmd *cobra.Command, args []string) error {
	ctx := context.Background()
	name := args[0]

	// Get area
	var areaID string
	if projectAreaFlag != "" {
		area, err := client.GetAreaBySlug(ctx, projectAreaFlag)
		if err != nil {
			return fmt.Errorf("area not found: %s", projectAreaFlag)
		}
		areaID = area.ID
	} else {
		// Interactive area selection
		areas, err := client.ListAreas(ctx)
		if err != nil {
			return fmt.Errorf("failed to list areas: %w", err)
		}

		if len(areas) == 0 {
			return fmt.Errorf("no areas found. Create one first with 'reorg area create <name>'")
		}

		fmt.Println("Select an area:")
		for i, a := range areas {
			fmt.Printf("  %d. %s\n", i+1, a.Title)
		}

		reader := bufio.NewReader(os.Stdin)
		fmt.Print("Enter number: ")
		input, _ := reader.ReadString('\n')
		input = strings.TrimSpace(input)

		num, err := strconv.Atoi(input)
		if err != nil || num < 1 || num > len(areas) {
			return fmt.Errorf("invalid selection")
		}

		areaID = areas[num-1].ID
	}

	// Create project
	project := domain.NewProject(name, areaID)

	// Set priority
	switch strings.ToLower(projectPriorityFlag) {
	case "low":
		project.Priority = domain.PriorityLow
	case "high":
		project.Priority = domain.PriorityHigh
	case "urgent":
		project.Priority = domain.PriorityUrgent
	default:
		project.Priority = domain.PriorityMedium
	}

	// Add tags
	for _, tag := range projectTagsFlag {
		project.AddTag(tag)
	}

	if _, err := client.CreateProject(ctx, project); err != nil {
		return fmt.Errorf("failed to create project: %w", err)
	}

	fmt.Printf("%s Created project: %s\n", successStyle.Render("✓"), name)
	return nil
}

func runProjectShow(cmd *cobra.Command, args []string) error {
	ctx := context.Background()
	slug := args[0]

	// Try to find project by slug (checking all areas)
	var project *domain.Project
	areas, _ := client.ListAreas(ctx)
	for _, area := range areas {
		p, err := client.GetProjectBySlug(ctx, area.ID, slug)
		if err == nil {
			project = p
			break
		}
	}

	if project == nil {
		return fmt.Errorf("project not found: %s", slug)
	}

	// Get area
	area, _ := client.GetArea(ctx, project.AreaID)
	areaName := ""
	if area != nil {
		areaName = area.Title
	}

	// Get tasks
	tasks, _ := client.ListTasks(ctx, project.ID)
	completedTasks := 0
	for _, t := range tasks {
		if t.IsComplete() {
			completedTasks++
		}
	}

	headerStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("12"))
	labelStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("8"))

	fmt.Println()
	fmt.Println(headerStyle.Render(project.Title))
	fmt.Println()

	fmt.Printf("%s %s\n", labelStyle.Render("ID:"), project.ID)
	fmt.Printf("%s %s\n", labelStyle.Render("Area:"), areaName)
	fmt.Printf("%s %s\n", labelStyle.Render("Status:"), project.Status)
	fmt.Printf("%s %s\n", labelStyle.Render("Priority:"), project.Priority)
	fmt.Printf("%s %s\n", labelStyle.Render("Created:"), project.Created.Format("2006-01-02 15:04"))
	fmt.Printf("%s %s\n", labelStyle.Render("Updated:"), project.Updated.Format("2006-01-02 15:04"))

	if project.DueDate != nil {
		fmt.Printf("%s %s\n", labelStyle.Render("Due:"), project.DueDate.Format("2006-01-02"))
	}

	if len(project.Tags) > 0 {
		fmt.Printf("%s %s\n", labelStyle.Render("Tags:"), strings.Join(project.Tags, ", "))
	}

	fmt.Println()
	fmt.Printf("%s %d/%d completed\n", labelStyle.Render("Tasks:"), completedTasks, len(tasks))
	fmt.Println()

	if project.Content != "" {
		fmt.Println(labelStyle.Render("Description:"))
		fmt.Println(project.Content)
		fmt.Println()
	}

	if len(tasks) > 0 {
		fmt.Println(labelStyle.Render("Tasks:"))
		for _, t := range tasks {
			statusIcon := "○"
			if t.IsComplete() {
				statusIcon = successStyle.Render("✓")
			} else if t.Status == domain.TaskStatusInProgress {
				statusIcon = "◐"
			} else if t.Status == domain.TaskStatusBlocked {
				statusIcon = "⊘"
			}
			fmt.Printf("  %s %s\n", statusIcon, t.Title)
		}
		fmt.Println()
	}

	return nil
}

func runProjectComplete(cmd *cobra.Command, args []string) error {
	ctx := context.Background()
	slug := args[0]

	// Find project
	var project *domain.Project
	areas, _ := client.ListAreas(ctx)
	for _, area := range areas {
		p, err := client.GetProjectBySlug(ctx, area.ID, slug)
		if err == nil {
			project = p
			break
		}
	}

	if project == nil {
		return fmt.Errorf("project not found: %s", slug)
	}

	if err := client.CompleteProject(ctx, project.ID); err != nil {
		return fmt.Errorf("failed to complete project: %w", err)
	}

	fmt.Printf("%s Completed project: %s\n", successStyle.Render("✓"), project.Title)
	return nil
}

func runProjectDelete(cmd *cobra.Command, args []string) error {
	ctx := context.Background()
	slug := args[0]

	// Find project
	var project *domain.Project
	areas, _ := client.ListAreas(ctx)
	for _, area := range areas {
		p, err := client.GetProjectBySlug(ctx, area.ID, slug)
		if err == nil {
			project = p
			break
		}
	}

	if project == nil {
		return fmt.Errorf("project not found: %s", slug)
	}

	// Check for tasks
	tasks, _ := client.ListTasks(ctx, project.ID)
	if len(tasks) > 0 {
		return fmt.Errorf("cannot delete project with %d tasks. Delete tasks first", len(tasks))
	}

	if err := client.DeleteProject(ctx, project.ID); err != nil {
		return fmt.Errorf("failed to delete project: %w", err)
	}

	fmt.Printf("%s Deleted project: %s\n", successStyle.Render("✓"), project.Title)
	return nil
}
