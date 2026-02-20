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
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/parquet-go/parquet-go"
	"github.com/parquet-go/parquet-go/compress/zstd"

	"github.com/louloulibs/wrds-download/internal/db"
)

// Options controls the export behaviour.
type Options struct {
	Format string // "parquet" or "csv"
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

	rows, err := conn.Query(ctx, query)
	if err != nil {
		return fmt.Errorf("query: %w", err)
	}
	defer rows.Close()

	switch format {
	case "csv":
		return writeCSV(rows, outPath)
	default:
		return writeParquet(rows, outPath)
	}
}

// writeCSV streams rows into a CSV file with a header row.
func writeCSV(rows pgx.Rows, outPath string) error {
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
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("rows: %w", err)
	}

	w.Flush()
	return w.Error()
}

// writeParquet streams rows into a Parquet file using parquet-go.
func writeParquet(rows pgx.Rows, outPath string) error {
	fds := rows.FieldDescriptions()

	schema, colTypes := buildParquetSchema(fds)

	f, err := os.Create(outPath)
	if err != nil {
		return fmt.Errorf("create parquet: %w", err)
	}
	defer f.Close()

	writer := parquet.NewGenericWriter[map[string]any](f,
		schema,
		parquet.Compression(&zstd.Codec{}),
		parquet.DefaultEncodingFor(parquet.ByteArray, &parquet.Plain),
	)

	buf := make([]map[string]any, 0, rowGroupSize)

	for rows.Next() {
		vals, err := rows.Values()
		if err != nil {
			return fmt.Errorf("scan row: %w", err)
		}

		row := make(map[string]any, len(fds))
		for i, v := range vals {
			row[fds[i].Name] = convertValue(v, colTypes[i])
		}
		buf = append(buf, row)

		if len(buf) >= rowGroupSize {
			if _, err := writer.Write(buf); err != nil {
				return fmt.Errorf("write row group: %w", err)
			}
			buf = buf[:0]
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("rows: %w", err)
	}

	// Flush remaining rows.
	if len(buf) > 0 {
		if _, err := writer.Write(buf); err != nil {
			return fmt.Errorf("write final rows: %w", err)
		}
	}

	return writer.Close()
}

// colType tags how we convert PG values for Parquet.
type colType int

const (
	colString colType = iota
	colBool
	colInt32
	colInt64
	colFloat32
	colFloat64
	colDate      // days since epoch → int32
	colTimestamp // microseconds since epoch → int64
)

// buildParquetSchema maps PG field descriptors to a parquet schema.
func buildParquetSchema(fds []pgconn.FieldDescription) (*parquet.Schema, []colType) {
	cols := make([]colType, len(fds))
	group := make(parquet.Group, len(fds))

	for i, fd := range fds {
		var node parquet.Node

		switch fd.DataTypeOID {
		case 16: // bool
			cols[i] = colBool
			node = parquet.Optional(parquet.Leaf(parquet.BooleanType))
		case 21: // int2
			cols[i] = colInt32
			node = parquet.Optional(parquet.Leaf(parquet.Int32Type))
		case 23: // int4
			cols[i] = colInt32
			node = parquet.Optional(parquet.Leaf(parquet.Int32Type))
		case 20: // int8
			cols[i] = colInt64
			node = parquet.Optional(parquet.Leaf(parquet.Int64Type))
		case 700: // float4
			cols[i] = colFloat32
			node = parquet.Optional(parquet.Leaf(parquet.FloatType))
		case 701: // float8
			cols[i] = colFloat64
			node = parquet.Optional(parquet.Leaf(parquet.DoubleType))
		case 1082: // date
			cols[i] = colDate
			node = parquet.Optional(parquet.Date())
		case 1114, 1184: // timestamp, timestamptz
			cols[i] = colTimestamp
			node = parquet.Optional(parquet.Timestamp(parquet.Microsecond))
		default:
			// text (25), varchar (1043), char (18, 1042), numeric (1700), etc.
			cols[i] = colString
			node = parquet.Optional(parquet.String())
		}

		group[fd.Name] = node
	}

	return parquet.NewSchema("wrds", group), cols
}

var epoch = time.Date(1970, 1, 1, 0, 0, 0, 0, time.UTC)

// convertValue converts a pgx-scanned value to the appropriate Go type for parquet-go.
func convertValue(v any, ct colType) any {
	if v == nil {
		return nil
	}

	switch ct {
	case colBool:
		if b, ok := v.(bool); ok {
			return b
		}
	case colInt32:
		switch n := v.(type) {
		case int16:
			return int32(n)
		case int32:
			return n
		case int64:
			return int32(n)
		}
	case colInt64:
		switch n := v.(type) {
		case int64:
			return n
		case int32:
			return int64(n)
		case int16:
			return int64(n)
		}
	case colFloat32:
		if f, ok := v.(float32); ok {
			return f
		}
		if f, ok := v.(float64); ok {
			return float32(f)
		}
	case colFloat64:
		if f, ok := v.(float64); ok {
			return f
		}
		if f, ok := v.(float32); ok {
			return float64(f)
		}
	case colDate:
		if t, ok := v.(time.Time); ok {
			days := int32(t.Sub(epoch).Hours() / 24)
			return days
		}
	case colTimestamp:
		if t, ok := v.(time.Time); ok {
			return t.Sub(epoch).Microseconds()
		}
	case colString:
		return formatValue(v)
	}

	// Fallback: stringify.
	return formatValue(v)
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
