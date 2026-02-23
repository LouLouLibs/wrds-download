package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"text/tabwriter"
	"time"

	"github.com/louloulibs/wrds-download/internal/config"
	"github.com/louloulibs/wrds-download/internal/db"
	"github.com/spf13/cobra"
)

var (
	infoSchema string
	infoTable  string
	infoJSON   bool
)

var infoCmd = &cobra.Command{
	Use:   "info",
	Short: "Show table metadata (columns, types, row count)",
	Long: `Display metadata for a WRDS table: comment, estimated row count,
size, and column details (name, type, nullable, description).

Examples:
  wrds-dl info --schema crsp --table dsf
  wrds-dl info --schema comp --table funda --json`,
	RunE: runInfo,
}

func init() {
	rootCmd.AddCommand(infoCmd)

	f := infoCmd.Flags()
	f.StringVar(&infoSchema, "schema", "", "Schema name (required)")
	f.StringVar(&infoTable, "table", "", "Table name (required)")
	f.BoolVar(&infoJSON, "json", false, "Output as JSON")

	_ = infoCmd.MarkFlagRequired("schema")
	_ = infoCmd.MarkFlagRequired("table")
}

func runInfo(cmd *cobra.Command, args []string) error {
	config.ApplyCredentials()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	client, err := db.New(ctx)
	if err != nil {
		return fmt.Errorf("connect: %w", err)
	}
	defer client.Close()

	meta, err := client.TableMeta(ctx, infoSchema, infoTable)
	if err != nil {
		return fmt.Errorf("table meta: %w", err)
	}

	if infoJSON {
		return printInfoJSON(meta)
	}
	return printInfoTable(meta)
}

type jsonColumn struct {
	Name        string `json:"name"`
	Type        string `json:"type"`
	Nullable    bool   `json:"nullable"`
	Description string `json:"description,omitempty"`
}

type jsonInfo struct {
	Schema   string       `json:"schema"`
	Table    string       `json:"table"`
	Comment  string       `json:"comment,omitempty"`
	RowCount int64        `json:"row_count"`
	Size     string       `json:"size,omitempty"`
	Columns  []jsonColumn `json:"columns"`
}

func printInfoJSON(meta *db.TableMeta) error {
	info := jsonInfo{
		Schema:   meta.Schema,
		Table:    meta.Table,
		Comment:  meta.Comment,
		RowCount: meta.RowCount,
		Size:     meta.Size,
		Columns:  make([]jsonColumn, len(meta.Columns)),
	}
	for i, c := range meta.Columns {
		info.Columns[i] = jsonColumn{
			Name:        c.Name,
			Type:        c.DataType,
			Nullable:    c.Nullable,
			Description: c.Description,
		}
	}

	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(info)
}

func printInfoTable(meta *db.TableMeta) error {
	fmt.Fprintf(os.Stdout, "%s.%s\n", meta.Schema, meta.Table)
	if meta.Comment != "" {
		fmt.Fprintf(os.Stdout, "  %s\n", meta.Comment)
	}
	if meta.RowCount > 0 || meta.Size != "" {
		parts := ""
		if meta.RowCount > 0 {
			parts += fmt.Sprintf("~%d rows", meta.RowCount)
		}
		if meta.Size != "" {
			if parts != "" {
				parts += ", "
			}
			parts += meta.Size
		}
		fmt.Fprintf(os.Stdout, "  %s\n", parts)
	}
	fmt.Fprintln(os.Stdout)

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "NAME\tTYPE\tNULLABLE\tDESCRIPTION")
	for _, c := range meta.Columns {
		nullable := "NO"
		if c.Nullable {
			nullable = "YES"
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", c.Name, c.DataType, nullable, c.Description)
	}
	return w.Flush()
}
