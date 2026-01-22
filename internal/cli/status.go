package cli

import (
	"context"
	"fmt"

	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"

	"github.com/ihavespoons/reorg/internal/domain"
)

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show an overview of your organization",
	Long:  `Display a summary of all areas, projects, and tasks.`,
	RunE:  runStatus,
}

func init() {
	rootCmd.AddCommand(statusCmd)
}

func runStatus(cmd *cobra.Command, args []string) error {
	ctx := context.Background()

	headerStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("12"))
	areaStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("14"))
	projectStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("15"))
	countStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("8"))

	fmt.Println()
	fmt.Println(headerStyle.Render("  Reorg Status"))
	fmt.Println()

	areas, err := client.ListAreas(ctx)
	if err != nil {
		return fmt.Errorf("failed to list areas: %w", err)
	}

	if len(areas) == 0 {
		fmt.Println("  No areas found. Run 'reorg init' to get started.")
		return nil
	}

	var totalProjects, totalTasks, completedTasks, inProgressTasks, overdueTasks int

	for _, area := range areas {
		projects, _ := client.ListProjects(ctx, area.ID)

		var areaTasksTotal, areaTasksComplete int

		fmt.Printf("  %s\n", areaStyle.Render(area.Title))

		if len(projects) == 0 {
			fmt.Println(countStyle.Render("    No projects"))
		} else {
			for _, p := range projects {
				tasks, _ := client.ListTasks(ctx, p.ID)

				var projectComplete, projectInProgress int
				for _, t := range tasks {
					totalTasks++
					areaTasksTotal++
					if t.IsComplete() {
						completedTasks++
						areaTasksComplete++
						projectComplete++
					} else if t.Status == domain.TaskStatusInProgress {
						inProgressTasks++
						projectInProgress++
					}
					if t.IsOverdue() {
						overdueTasks++
					}
				}

				// Status indicator
				statusIndicator := "○"
				if p.Status == domain.ProjectStatusCompleted {
					statusIndicator = successStyle.Render("✓")
				} else if p.Status == domain.ProjectStatusOnHold {
					statusIndicator = dimStyle.Render("⏸")
				} else if projectInProgress > 0 {
					statusIndicator = "◐"
				}

				taskInfo := ""
				if len(tasks) > 0 {
					taskInfo = countStyle.Render(fmt.Sprintf(" [%d/%d]", projectComplete, len(tasks)))
				}

				fmt.Printf("    %s %s%s\n", statusIndicator, projectStyle.Render(p.Title), taskInfo)
				totalProjects++
			}
		}

		// Area summary
		if areaTasksTotal > 0 {
			fmt.Println(countStyle.Render(fmt.Sprintf("    %d/%d tasks complete\n", areaTasksComplete, areaTasksTotal)))
		} else {
			fmt.Println()
		}
	}

	// Overall summary
	fmt.Println(lipgloss.NewStyle().Foreground(lipgloss.Color("8")).Render("  ─────────────────────────"))
	fmt.Println()
	fmt.Printf("  %s %d  %s %d  %s %d/%d\n",
		countStyle.Render("Areas:"), len(areas),
		countStyle.Render("Projects:"), totalProjects,
		countStyle.Render("Tasks:"), completedTasks, totalTasks,
	)

	if inProgressTasks > 0 {
		fmt.Printf("  %s %d in progress\n", countStyle.Render("Active:"), inProgressTasks)
	}

	if overdueTasks > 0 {
		overdueStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("9"))
		fmt.Printf("  %s\n", overdueStyle.Render(fmt.Sprintf("⚠ %d overdue tasks", overdueTasks)))
	}

	fmt.Println()

	return nil
}
