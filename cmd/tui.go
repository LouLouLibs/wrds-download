package cmd

import (
	"context"
	"fmt"
	"os"

	"github.com/eloualiche/wrds-download/internal/db"
	"github.com/eloualiche/wrds-download/internal/tui"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/spf13/cobra"
)

var tuiCmd = &cobra.Command{
	Use:   "tui",
	Short: "Launch the interactive TUI browser",
	RunE:  runTUI,
}

func init() {
	rootCmd.AddCommand(tuiCmd)
}

func runTUI(cmd *cobra.Command, args []string) error {
	ctx := context.Background()
	client, err := db.New(ctx)
	if err != nil {
		return fmt.Errorf("connect to WRDS: %w", err)
	}
	defer client.Close()

	m := tui.NewApp(client)
	p := tea.NewProgram(m, tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	return nil
}
