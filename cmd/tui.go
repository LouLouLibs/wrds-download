package cmd

import (
	"context"
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/eloualiche/wrds-download/internal/config"
	"github.com/eloualiche/wrds-download/internal/db"
	"github.com/eloualiche/wrds-download/internal/tui"
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
	config.ApplyCredentials()

	ctx := context.Background()
	client, err := db.New(ctx)
	if err != nil {
		// Launch TUI in login mode instead of crashing
		m := tui.NewAppNoClient()
		p := tea.NewProgram(m, tea.WithAltScreen())
		final, err := p.Run()
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		if a, ok := final.(*tui.App); ok && a.Err() != "" {
			fmt.Fprintln(os.Stderr, a.Err())
		}
		return nil
	}
	defer client.Close()

	m := tui.NewApp(client)
	p := tea.NewProgram(m, tea.WithAltScreen())
	final, err := p.Run()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if a, ok := final.(*tui.App); ok && a.Err() != "" {
		fmt.Fprintln(os.Stderr, a.Err())
	}
	return nil
}
