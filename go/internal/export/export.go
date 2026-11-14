package export

import (
	"context"
	"encoding/csv"
	"fmt"
	"math/big"
	"os"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/louloulibs/wrds-download/internal/db"
)

// Options controls the export behaviour.
type Options struct {
	Format       string         // "parquet" or "csv"
	ProgressFunc func(rows int) // called periodically with total rows exported so far
}

const rowGroupSize = 10_000

// Export runs query against the WRDS PostgreSQL instance and writes output to outPath.
// Format is determined by opts.Format (default: parquet).
func Export(query, outPath string, opts Options) error {
	format := strings.ToLower(opts.Format)
	if format == "" {
		if strings.HasSuffix(strings.ToLower(outPath), ".csv") {
			format = "csv"
		} else {
			format = "parquet"
		}
	}

	dsn, err := db.DSNFromEnv()
	if err != nil {
		return fmt.Errorf("dsn: %w", err)
	}

	ctx := context.Background()
	conn, err := pgx.Connect(ctx, dsn)
	if err != nil {
		return fmt.Errorf("connect: %w", err)
	}
	defer conn.Close(ctx)

	var args []any
	if format != "csv" {
		args = append(args, parquetResultFormats)
	}
	rows, err := conn.Query(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("query: %w", err)
	}
	defer rows.Close()

	switch format {
	case "csv":
		return writeCSV(rows, outPath, opts.ProgressFunc)
	default:
		return writeParquet(rows, outPath, opts.ProgressFunc)
	}
}

// writeCSV streams rows into a CSV file with a header row.
func writeCSV(rows pgx.Rows, outPath string, progressFn func(int)) error {
	f, err := os.Create(outPath)
	if err != nil {
		return fmt.Errorf("create csv: %w", err)
	}
	defer f.Close()

	w := csv.NewWriter(f)
	defer w.Flush()

	fds := rows.FieldDescriptions()
	header := make([]string, len(fds))
	for i, fd := range fds {
		header[i] = fd.Name
	}
	if err := w.Write(header); err != nil {
		return fmt.Errorf("write header: %w", err)
	}

	record := make([]string, len(fds))
	total := 0
	for rows.Next() {
		vals, err := rows.Values()
		if err != nil {
			return fmt.Errorf("scan row: %w", err)
		}
		for i, v := range vals {
			record[i] = formatValue(v)
		}
		if err := w.Write(record); err != nil {
			return fmt.Errorf("write row: %w", err)
		}
		total++
		if progressFn != nil && total%rowGroupSize == 0 {
			progressFn(total)
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("rows: %w", err)
	}

	w.Flush()
	return w.Error()
}

// formatValue converts any value to its string representation.
func formatValue(v any) string {
	if v == nil {
		return ""
	}
	switch val := v.(type) {
	case string:
		return val
	case []byte:
		return string(val)
	case time.Time:
		if val.Hour() == 0 && val.Minute() == 0 && val.Second() == 0 && val.Nanosecond() == 0 {
			return val.Format("2006-01-02")
		}
		return val.Format(time.RFC3339)
	case pgtype.Numeric:
		if !val.Valid {
			return ""
		}
		if val.NaN {
			return "NaN"
		}
		if val.InfinityModifier == pgtype.Infinity {
			return "Infinity"
		}
		if val.InfinityModifier == pgtype.NegativeInfinity {
			return "-Infinity"
		}
		// Convert to big.Float for string representation.
		bi := val.Int
		if bi == nil {
			bi = new(big.Int)
		}
		bf := new(big.Float).SetInt(bi)
		if val.Exp < 0 {
			divisor := new(big.Float).SetInt(new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(-val.Exp)), nil))
			bf.Quo(bf, divisor)
		} else if val.Exp > 0 {
			multiplier := new(big.Float).SetInt(new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(val.Exp)), nil))
			bf.Mul(bf, multiplier)
		}
		return bf.Text('f', -1)
	default:
		return fmt.Sprintf("%v", val)
	}
}
