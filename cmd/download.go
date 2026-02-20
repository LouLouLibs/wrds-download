package cmd

import (
	"context"
	"fmt"
	"os"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/louloulibs/wrds-download/internal/config"
	"github.com/louloulibs/wrds-download/internal/db"
	"github.com/louloulibs/wrds-download/internal/export"
	"github.com/spf13/cobra"
)

var (
	dlSchema string
	dlTable  string
	dlColumns string
	dlWhere  string
	dlQuery  string
	dlOut    string
	dlFormat string
	dlLimit  int
	dlDryRun bool
)

var downloadCmd = &cobra.Command{
	Use:   "download",
	Short: "Download WRDS data to parquet or CSV",
	Long: `Download data from WRDS to a local file.

Examples:
  wrds download --schema crsp --table dsf --where "date='2020-01-02'" --out crsp_dsf.parquet
  wrds download --schema comp --table funda --columns "gvkey,datadate,sale" --out funda.parquet
  wrds download --query "SELECT permno, date, prc FROM crsp.dsf LIMIT 1000" --out out.parquet
  wrds download --schema comp --table funda --out funda.csv --format csv`,
	RunE: runDownload,
}

func init() {
	rootCmd.AddCommand(downloadCmd)

	f := downloadCmd.Flags()
	f.StringVar(&dlSchema, "schema", "", "Schema name (e.g. crsp)")
	f.StringVar(&dlTable, "table", "", "Table name (e.g. dsf)")
	f.StringVarP(&dlColumns, "columns", "c", "*", "Columns to select (comma-separated, default *)")
	f.StringVar(&dlWhere, "where", "", "SQL WHERE clause (without the WHERE keyword)")
	f.StringVar(&dlQuery, "query", "", "Full SQL query (overrides --schema/--table/--where)")
	f.StringVar(&dlOut, "out", "", "Output file path (required)")
	f.StringVar(&dlFormat, "format", "", "Output format: parquet or csv (inferred from extension if omitted)")
	f.IntVar(&dlLimit, "limit", 0, "Limit number of rows (0 = no limit)")
	f.BoolVar(&dlDryRun, "dry-run", false, "Preview the query, row count, and first 5 rows without downloading")
}

func runDownload(cmd *cobra.Command, args []string) error {
	config.ApplyCredentials()

	query, err := buildQuery()
	if err != nil {
		return err
	}

	if dlDryRun {
		return runDryRun(query)
	}

	if dlOut == "" {
		return fmt.Errorf("required flag \"out\" not set")
	}

	format := resolveFormat(dlOut, dlFormat)

	fmt.Fprintf(os.Stderr, "Exporting to %s (%s)...\n", dlOut, format)

	opts := export.Options{
		Format: format,
		ProgressFunc: func(rows int) {
			fmt.Fprintf(os.Stderr, "Exported %d rows...\n", rows)
		},
	}
	if err := export.Export(query, dlOut, opts); err != nil {
		return fmt.Errorf("export failed: %w", err)
	}

	fmt.Fprintf(os.Stderr, "Done: %s\n", dlOut)
	return nil
}

func runDryRun(query string) error {
	dsn, err := db.DSNFromEnv()
	if err != nil {
		return fmt.Errorf("dsn: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	conn, err := pgx.Connect(ctx, dsn)
	if err != nil {
		return fmt.Errorf("connect: %w", err)
	}
	defer conn.Close(ctx)

	fmt.Fprintln(os.Stdout, "Query:")
	fmt.Fprintln(os.Stdout, " ", query)
	fmt.Fprintln(os.Stdout)

	// Row count
	countQuery := fmt.Sprintf("SELECT count(*) FROM (%s) sub", query)
	var count int64
	if err := conn.QueryRow(ctx, countQuery).Scan(&count); err != nil {
		return fmt.Errorf("count query: %w", err)
	}
	fmt.Fprintf(os.Stdout, "Row count: %d\n\n", count)

	// Preview first 5 rows
	previewQuery := fmt.Sprintf("SELECT * FROM (%s) sub LIMIT 5", query)
	rows, err := conn.Query(ctx, previewQuery)
	if err != nil {
		return fmt.Errorf("preview query: %w", err)
	}
	defer rows.Close()

	fds := rows.FieldDescriptions()
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)

	// Header
	headers := make([]string, len(fds))
	for i, fd := range fds {
		headers[i] = fd.Name
	}
	fmt.Fprintln(w, strings.Join(headers, "\t"))

	// Rows
	for rows.Next() {
		vals, err := rows.Values()
		if err != nil {
			return fmt.Errorf("scan row: %w", err)
		}
		cells := make([]string, len(vals))
		for i, v := range vals {
			if v == nil {
				cells[i] = "NULL"
			} else {
				cells[i] = fmt.Sprintf("%v", v)
			}
		}
		fmt.Fprintln(w, strings.Join(cells, "\t"))
	}
	return w.Flush()
}

func buildQuery() (string, error) {
	if dlQuery != "" {
		return dlQuery, nil
	}
	if dlSchema == "" || dlTable == "" {
		return "", fmt.Errorf("either --query or both --schema and --table must be specified")
	}

	sel := "*"
	if dlColumns != "" && dlColumns != "*" {
		parts := strings.Split(dlColumns, ",")
		quoted := make([]string, len(parts))
		for i, p := range parts {
			quoted[i] = db.QuoteIdent(strings.TrimSpace(p))
		}
		sel = strings.Join(quoted, ", ")
	}
	q := fmt.Sprintf("SELECT %s FROM %s.%s", sel, db.QuoteIdent(dlSchema), db.QuoteIdent(dlTable))

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
