package cmd

import (
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/louloulibs/wrds-download/internal/config"
	"github.com/louloulibs/wrds-download/internal/tui"
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

	// Always start in login mode so the user controls when the
	// connection (and any 2FA prompt) happens.
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
