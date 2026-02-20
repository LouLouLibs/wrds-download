package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/eloualiche/wrds-download/internal/config"
	"github.com/eloualiche/wrds-download/internal/export"
	"github.com/spf13/cobra"
)

var (
	dlSchema string
	dlTable  string
	dlWhere  string
	dlQuery  string
	dlOut    string
	dlFormat string
	dlLimit  int
)

var downloadCmd = &cobra.Command{
	Use:   "download",
	Short: "Download WRDS data to parquet or CSV",
	Long: `Download data from WRDS to a local file.

Examples:
  wrds download --schema crsp --table dsf --where "date='2020-01-02'" --out crsp_dsf.parquet
  wrds download --query "SELECT permno, date, prc FROM crsp.dsf LIMIT 1000" --out out.parquet
  wrds download --schema comp --table funda --out funda.csv --format csv`,
	RunE: runDownload,
}

func init() {
	rootCmd.AddCommand(downloadCmd)

	f := downloadCmd.Flags()
	f.StringVar(&dlSchema, "schema", "", "Schema name (e.g. crsp)")
	f.StringVar(&dlTable, "table", "", "Table name (e.g. dsf)")
	f.StringVar(&dlWhere, "where", "", "SQL WHERE clause (without the WHERE keyword)")
	f.StringVar(&dlQuery, "query", "", "Full SQL query (overrides --schema/--table/--where)")
	f.StringVar(&dlOut, "out", "", "Output file path (required)")
	f.StringVar(&dlFormat, "format", "", "Output format: parquet or csv (inferred from extension if omitted)")
	f.IntVar(&dlLimit, "limit", 0, "Limit number of rows (0 = no limit)")

	_ = downloadCmd.MarkFlagRequired("out")
}

func runDownload(cmd *cobra.Command, args []string) error {
	config.ApplyCredentials()

	query, err := buildQuery()
	if err != nil {
		return err
	}

	format := resolveFormat(dlOut, dlFormat)

	fmt.Fprintf(os.Stderr, "Exporting to %s (%s)...\n", dlOut, format)

	opts := export.Options{Format: format}
	if err := export.Export(query, dlOut, opts); err != nil {
		return fmt.Errorf("export failed: %w", err)
	}

	fmt.Fprintf(os.Stderr, "Done: %s\n", dlOut)
	return nil
}

func buildQuery() (string, error) {
	if dlQuery != "" {
		return dlQuery, nil
	}
	if dlSchema == "" || dlTable == "" {
		return "", fmt.Errorf("either --query or both --schema and --table must be specified")
	}

	q := fmt.Sprintf("SELECT * FROM wrds.%s.%s", dlSchema, dlTable)

	if dlWhere != "" {
		q += " WHERE " + dlWhere
	}
	if dlLimit > 0 {
		q += fmt.Sprintf(" LIMIT %d", dlLimit)
	}

	return q, nil
}

func resolveFormat(path, flag string) string {
	if flag != "" {
		return strings.ToLower(flag)
	}
	lower := strings.ToLower(path)
	if strings.HasSuffix(lower, ".csv") {
		return "csv"
	}
	return "parquet"
}
