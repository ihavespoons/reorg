package cli

import (
	"context"
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"

	"github.com/beng/reorg/internal/domain"
)

var areaCmd = &cobra.Command{
	Use:   "area",
	Short: "Manage areas",
	Long:  `Areas are top-level categories for organizing projects (e.g., Work, Personal, Life Admin).`,
}

var areaListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all areas",
	RunE:  runAreaList,
}

var areaCreateCmd = &cobra.Command{
	Use:   "create [name]",
	Short: "Create a new area",
	Args:  cobra.ExactArgs(1),
	RunE:  runAreaCreate,
}

var areaShowCmd = &cobra.Command{
	Use:   "show [name]",
	Short: "Show area details",
	Args:  cobra.ExactArgs(1),
	RunE:  runAreaShow,
}

var areaDeleteCmd = &cobra.Command{
	Use:   "delete [name]",
	Short: "Delete an area",
	Args:  cobra.ExactArgs(1),
	RunE:  runAreaDelete,
}

func init() {
	rootCmd.AddCommand(areaCmd)
	areaCmd.AddCommand(areaListCmd)
	areaCmd.AddCommand(areaCreateCmd)
	areaCmd.AddCommand(areaShowCmd)
	areaCmd.AddCommand(areaDeleteCmd)
}

func runAreaList(cmd *cobra.Command, args []string) error {
	ctx := context.Background()

	areas, err := client.ListAreas(ctx)
	if err != nil {
		return fmt.Errorf("failed to list areas: %w", err)
	}

	if len(areas) == 0 {
		fmt.Println("No areas found. Create one with 'reorg area create <name>'")
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	_, _ = fmt.Fprintln(w, "NAME\tPROJECTS\tCREATED")
	_, _ = fmt.Fprintln(w, "----\t--------\t-------")

	for _, area := range areas {
		// Count projects
		projects, _ := client.ListProjects(ctx, area.ID)
		projectCount := len(projects)

		_, _ = fmt.Fprintf(w, "%s\t%d\t%s\n",
			area.Title,
			projectCount,
			area.Created.Format("2006-01-02"),
		)
	}

	return w.Flush()
}

func runAreaCreate(cmd *cobra.Command, args []string) error {
	ctx := context.Background()
	name := args[0]

	area := domain.NewArea(name)
	if _, err := client.CreateArea(ctx, area); err != nil {
		return fmt.Errorf("failed to create area: %w", err)
	}

	fmt.Printf("%s Created area: %s\n", successStyle.Render("✓"), name)
	return nil
}

func runAreaShow(cmd *cobra.Command, args []string) error {
	ctx := context.Background()
	slug := args[0]

	area, err := client.GetAreaBySlug(ctx, slug)
	if err != nil {
		return fmt.Errorf("area not found: %s", slug)
	}

	// Count projects and tasks
	projects, _ := client.ListProjects(ctx, area.ID)
	var totalTasks, completedTasks int
	for _, p := range projects {
		tasks, _ := client.ListTasks(ctx, p.ID)
		for _, t := range tasks {
			totalTasks++
			if t.IsComplete() {
				completedTasks++
			}
		}
	}

	headerStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("12"))
	labelStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("8"))

	fmt.Println()
	fmt.Println(headerStyle.Render(area.Title))
	if area.Icon != "" || area.Color != "" {
		fmt.Printf("%s %s  %s %s\n",
			labelStyle.Render("Icon:"), area.Icon,
			labelStyle.Render("Color:"), area.Color,
		)
	}
	fmt.Println()

	fmt.Printf("%s %s\n", labelStyle.Render("ID:"), area.ID)
	fmt.Printf("%s %s\n", labelStyle.Render("Created:"), area.Created.Format("2006-01-02 15:04"))
	fmt.Printf("%s %s\n", labelStyle.Render("Updated:"), area.Updated.Format("2006-01-02 15:04"))
	fmt.Println()

	fmt.Printf("%s %d\n", labelStyle.Render("Projects:"), len(projects))
	fmt.Printf("%s %d/%d completed\n", labelStyle.Render("Tasks:"), completedTasks, totalTasks)
	fmt.Println()

	if area.Content != "" {
		fmt.Println(labelStyle.Render("Description:"))
		fmt.Println(area.Content)
		fmt.Println()
	}

	if len(projects) > 0 {
		fmt.Println(labelStyle.Render("Projects:"))
		for _, p := range projects {
			statusIcon := "○"
			switch p.Status {
			case domain.ProjectStatusCompleted:
				statusIcon = "✓"
			case domain.ProjectStatusOnHold:
				statusIcon = "⏸"
			}
			fmt.Printf("  %s %s\n", statusIcon, p.Title)
		}
		fmt.Println()
	}

	return nil
}

func runAreaDelete(cmd *cobra.Command, args []string) error {
	ctx := context.Background()
	slug := args[0]

	area, err := client.GetAreaBySlug(ctx, slug)
	if err != nil {
		return fmt.Errorf("area not found: %s", slug)
	}

	// Check for projects
	projects, _ := client.ListProjects(ctx, area.ID)
	if len(projects) > 0 {
		return fmt.Errorf("cannot delete area with %d projects. Delete projects first", len(projects))
	}

	if err := client.DeleteArea(ctx, area.ID); err != nil {
		return fmt.Errorf("failed to delete area: %w", err)
	}

	fmt.Printf("%s Deleted area: %s\n", successStyle.Render("✓"), area.Title)
	return nil
}
