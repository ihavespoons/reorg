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
	taskProjectFlag  string
	taskPriorityFlag string
	taskTagsFlag     []string
	taskStatusFlag   string
)

var taskCmd = &cobra.Command{
	Use:   "task",
	Short: "Manage tasks",
	Long:  `Tasks are individual actionable items within a project.`,
}

var taskListCmd = &cobra.Command{
	Use:   "list",
	Short: "List tasks",
	RunE:  runTaskList,
}

var taskCreateCmd = &cobra.Command{
	Use:   "create [title]",
	Short: "Create a new task",
	Args:  cobra.ExactArgs(1),
	RunE:  runTaskCreate,
}

var taskShowCmd = &cobra.Command{
	Use:   "show [task-id]",
	Short: "Show task details",
	Args:  cobra.ExactArgs(1),
	RunE:  runTaskShow,
}

var taskCompleteCmd = &cobra.Command{
	Use:   "complete [task-id]",
	Short: "Mark a task as completed",
	Args:  cobra.ExactArgs(1),
	RunE:  runTaskComplete,
}

var taskStartCmd = &cobra.Command{
	Use:   "start [task-id]",
	Short: "Mark a task as in progress",
	Args:  cobra.ExactArgs(1),
	RunE:  runTaskStart,
}

var taskDeleteCmd = &cobra.Command{
	Use:   "delete [task-id]",
	Short: "Delete a task",
	Args:  cobra.ExactArgs(1),
	RunE:  runTaskDelete,
}

func init() {
	rootCmd.AddCommand(taskCmd)
	taskCmd.AddCommand(taskListCmd)
	taskCmd.AddCommand(taskCreateCmd)
	taskCmd.AddCommand(taskShowCmd)
	taskCmd.AddCommand(taskCompleteCmd)
	taskCmd.AddCommand(taskStartCmd)
	taskCmd.AddCommand(taskDeleteCmd)

	// List flags
	taskListCmd.Flags().StringVarP(&taskProjectFlag, "project", "p", "", "Filter by project")
	taskListCmd.Flags().StringVarP(&taskStatusFlag, "status", "s", "", "Filter by status (pending, in_progress, completed, blocked)")

	// Create flags
	taskCreateCmd.Flags().StringVarP(&taskProjectFlag, "project", "p", "", "Project for the task")
	taskCreateCmd.Flags().StringVar(&taskPriorityFlag, "priority", "medium", "Priority (low, medium, high, urgent)")
	taskCreateCmd.Flags().StringSliceVarP(&taskTagsFlag, "tags", "t", nil, "Tags for the task")
}

func runTaskList(cmd *cobra.Command, args []string) error {
	ctx := context.Background()

	var tasks []*domain.Task
	var err error

	if taskProjectFlag != "" {
		// Find project by slug
		var project *domain.Project
		areas, _ := client.ListAreas(ctx)
		for _, area := range areas {
			p, err := client.GetProjectBySlug(ctx, area.ID, taskProjectFlag)
			if err == nil {
				project = p
				break
			}
		}
		if project == nil {
			return fmt.Errorf("project not found: %s", taskProjectFlag)
		}
		tasks, err = client.ListTasks(ctx, project.ID)
	} else {
		tasks, err = client.ListAllTasks(ctx)
	}

	if err != nil {
		return fmt.Errorf("failed to list tasks: %w", err)
	}

	// Filter by status if specified
	if taskStatusFlag != "" {
		var filtered []*domain.Task
		for _, t := range tasks {
			if string(t.Status) == taskStatusFlag {
				filtered = append(filtered, t)
			}
		}
		tasks = filtered
	}

	if len(tasks) == 0 {
		fmt.Println("No tasks found. Create one with 'reorg task create <title>'")
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "STATUS\tTASK\tPROJECT\tPRIORITY\tDUE")
	fmt.Fprintln(w, "------\t----\t-------\t--------\t---")

	for _, t := range tasks {
		// Get project name
		project, _ := client.GetProject(ctx, t.ProjectID)
		projectName := ""
		if project != nil {
			projectName = project.Title
		}

		// Status icon
		statusIcon := "○"
		switch t.Status {
		case domain.TaskStatusCompleted:
			statusIcon = "✓"
		case domain.TaskStatusInProgress:
			statusIcon = "◐"
		case domain.TaskStatusBlocked:
			statusIcon = "⊘"
		case domain.TaskStatusCancelled:
			statusIcon = "✗"
		}

		// Due date
		dueStr := "-"
		if t.DueDate != nil {
			dueStr = t.DueDate.Format("2006-01-02")
			if t.IsOverdue() {
				dueStr = lipgloss.NewStyle().Foreground(lipgloss.Color("9")).Render(dueStr + " (overdue)")
			}
		}

		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n",
			statusIcon,
			t.Title,
			projectName,
			t.Priority,
			dueStr,
		)
	}

	w.Flush()
	return nil
}

func runTaskCreate(cmd *cobra.Command, args []string) error {
	ctx := context.Background()
	title := args[0]

	// Get project
	var projectID, areaID string

	if taskProjectFlag != "" {
		// Find project by slug
		areas, _ := client.ListAreas(ctx)
		for _, area := range areas {
			p, err := client.GetProjectBySlug(ctx, area.ID, taskProjectFlag)
			if err == nil {
				projectID = p.ID
				areaID = p.AreaID
				break
			}
		}
		if projectID == "" {
			return fmt.Errorf("project not found: %s", taskProjectFlag)
		}
	} else {
		// Interactive project selection
		projects, err := client.ListAllProjects(ctx)
		if err != nil {
			return fmt.Errorf("failed to list projects: %w", err)
		}

		if len(projects) == 0 {
			return fmt.Errorf("no projects found. Create one first with 'reorg project create <name>'")
		}

		fmt.Println("Select a project:")
		for i, p := range projects {
			area, _ := client.GetArea(ctx, p.AreaID)
			areaName := ""
			if area != nil {
				areaName = area.Title + "/"
			}
			fmt.Printf("  %d. %s%s\n", i+1, areaName, p.Title)
		}

		reader := bufio.NewReader(os.Stdin)
		fmt.Print("Enter number: ")
		input, _ := reader.ReadString('\n')
		input = strings.TrimSpace(input)

		num, err := strconv.Atoi(input)
		if err != nil || num < 1 || num > len(projects) {
			return fmt.Errorf("invalid selection")
		}

		projectID = projects[num-1].ID
		areaID = projects[num-1].AreaID
	}

	// Create task
	task := domain.NewTask(title, projectID, areaID)

	// Set priority
	switch strings.ToLower(taskPriorityFlag) {
	case "low":
		task.Priority = domain.PriorityLow
	case "high":
		task.Priority = domain.PriorityHigh
	case "urgent":
		task.Priority = domain.PriorityUrgent
	default:
		task.Priority = domain.PriorityMedium
	}

	// Add tags
	for _, tag := range taskTagsFlag {
		task.AddTag(tag)
	}

	created, err := client.CreateTask(ctx, task)
	if err != nil {
		return fmt.Errorf("failed to create task: %w", err)
	}

	fmt.Printf("%s Created task: %s\n", successStyle.Render("✓"), title)
	fmt.Printf("  ID: %s\n", dimStyle.Render(created.ID))
	return nil
}

func runTaskShow(cmd *cobra.Command, args []string) error {
	ctx := context.Background()
	taskID := args[0]

	// Try to find by ID first, then by slug
	task, err := client.GetTask(ctx, taskID)
	if err != nil {
		// Try finding by slug in all tasks
		tasks, _ := client.ListAllTasks(ctx)
		for _, t := range tasks {
			if t.Slug() == taskID || strings.HasPrefix(t.ID, taskID) {
				task = t
				break
			}
		}
	}

	if task == nil {
		return fmt.Errorf("task not found: %s", taskID)
	}

	// Get project and area
	project, _ := client.GetProject(ctx, task.ProjectID)
	area, _ := client.GetArea(ctx, task.AreaID)

	projectName := ""
	if project != nil {
		projectName = project.Title
	}
	areaName := ""
	if area != nil {
		areaName = area.Title
	}

	headerStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("12"))
	labelStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("8"))

	fmt.Println()
	fmt.Println(headerStyle.Render(task.Title))
	fmt.Println()

	fmt.Printf("%s %s\n", labelStyle.Render("ID:"), task.ID)
	fmt.Printf("%s %s / %s\n", labelStyle.Render("Location:"), areaName, projectName)
	fmt.Printf("%s %s\n", labelStyle.Render("Status:"), task.Status)
	fmt.Printf("%s %s\n", labelStyle.Render("Priority:"), task.Priority)
	fmt.Printf("%s %s\n", labelStyle.Render("Created:"), task.Created.Format("2006-01-02 15:04"))
	fmt.Printf("%s %s\n", labelStyle.Render("Updated:"), task.Updated.Format("2006-01-02 15:04"))

	if task.DueDate != nil {
		dueStr := task.DueDate.Format("2006-01-02")
		if task.IsOverdue() {
			dueStr = lipgloss.NewStyle().Foreground(lipgloss.Color("9")).Render(dueStr + " (OVERDUE)")
		}
		fmt.Printf("%s %s\n", labelStyle.Render("Due:"), dueStr)
	}

	if task.TimeEstimate != "" {
		fmt.Printf("%s %s\n", labelStyle.Render("Estimate:"), task.TimeEstimate)
	}
	if task.TimeSpent != "" {
		fmt.Printf("%s %s\n", labelStyle.Render("Time Spent:"), task.TimeSpent)
	}

	if len(task.Tags) > 0 {
		fmt.Printf("%s %s\n", labelStyle.Render("Tags:"), strings.Join(task.Tags, ", "))
	}

	if len(task.Dependencies) > 0 {
		fmt.Printf("%s %s\n", labelStyle.Render("Dependencies:"), strings.Join(task.Dependencies, ", "))
	}

	fmt.Println()

	if task.Content != "" {
		fmt.Println(labelStyle.Render("Notes:"))
		fmt.Println(task.Content)
		fmt.Println()
	}

	return nil
}

func runTaskComplete(cmd *cobra.Command, args []string) error {
	ctx := context.Background()
	taskID := args[0]

	task, err := findTask(ctx, taskID)
	if err != nil {
		return err
	}

	if err := client.CompleteTask(ctx, task.ID); err != nil {
		return fmt.Errorf("failed to complete task: %w", err)
	}

	fmt.Printf("%s Completed: %s\n", successStyle.Render("✓"), task.Title)
	return nil
}

func runTaskStart(cmd *cobra.Command, args []string) error {
	ctx := context.Background()
	taskID := args[0]

	task, err := findTask(ctx, taskID)
	if err != nil {
		return err
	}

	if err := client.StartTask(ctx, task.ID); err != nil {
		return fmt.Errorf("failed to start task: %w", err)
	}

	fmt.Printf("%s Started: %s\n", successStyle.Render("◐"), task.Title)
	return nil
}

func runTaskDelete(cmd *cobra.Command, args []string) error {
	ctx := context.Background()
	taskID := args[0]

	task, err := findTask(ctx, taskID)
	if err != nil {
		return err
	}

	if err := client.DeleteTask(ctx, task.ID); err != nil {
		return fmt.Errorf("failed to delete task: %w", err)
	}

	fmt.Printf("%s Deleted: %s\n", successStyle.Render("✓"), task.Title)
	return nil
}

// findTask looks up a task by ID or partial ID/slug
func findTask(ctx context.Context, identifier string) (*domain.Task, error) {
	// Try exact ID first
	task, err := client.GetTask(ctx, identifier)
	if err == nil {
		return task, nil
	}

	// Try partial match
	tasks, _ := client.ListAllTasks(ctx)
	for _, t := range tasks {
		if t.Slug() == identifier || strings.HasPrefix(t.ID, identifier) {
			return t, nil
		}
	}

	return nil, fmt.Errorf("task not found: %s", identifier)
}
