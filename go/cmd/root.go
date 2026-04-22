package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

// Version is the CLI version. Overridable at build time via:
//   go build -ldflags "-X github.com/louloulibs/wrds-download/cmd.Version=v1.2.3"
var Version = "dev"

var rootCmd = &cobra.Command{
	Use:     "wrds-dl",
	Version: Version,
	Short:   "WRDS data browser and downloader",
	Long: `wrds is a CLI/TUI tool for navigating and downloading data
from the Wharton Research Data Services (WRDS) PostgreSQL database.

Authentication is read from standard PostgreSQL environment variables:
  PGHOST, PGPORT, PGUSER, PGPASSWORD, PGDATABASE

Or from ~/.pgpass if PGPASSWORD is not set.`,
}

// Execute runs the root command.
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
