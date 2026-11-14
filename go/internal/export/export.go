package export

import (
	"context"
	"encoding/csv"
	"fmt"
	"math/big"
	"os"
	"reflect"
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

	rows, err := conn.Query(ctx, query)
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

// writeParquet streams rows into a Parquet file using parquet-go.
//
// Rows are built as parquet.Row values with explicit definition levels so that
// only a SQL NULL is written as null: going through Go values (e.g. map[string]any)
// makes parquet-go treat zero values (0, 0.0, false, "") of optional columns as null.
func writeParquet(rows pgx.Rows, outPath string, progressFn func(int)) error {
	fds := rows.FieldDescriptions()

	schema, cols := buildParquetSchema(fds)

	f, err := os.Create(outPath)
	if err != nil {
		return fmt.Errorf("create parquet: %w", err)
	}
	defer f.Close()

	writer := parquet.NewWriter(f,
		schema,
		parquet.Compression(&zstd.Codec{}),
		parquet.DefaultEncodingFor(parquet.ByteArray, &parquet.Plain),
	)

	buf := make([]parquet.Row, 0, rowGroupSize)
	total := 0

	for rows.Next() {
		vals, err := rows.Values()
		if err != nil {
			return fmt.Errorf("scan row: %w", err)
		}

		row := make(parquet.Row, len(fds))
		for i, v := range vals {
			row[i] = parquetValue(v, cols[i], i)
		}
		buf = append(buf, row)

		if len(buf) >= rowGroupSize {
			if _, err := writer.WriteRows(buf); err != nil {
				return fmt.Errorf("write row group: %w", err)
			}
			total += len(buf)
			buf = buf[:0]
			if progressFn != nil {
				progressFn(total)
			}
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("rows: %w", err)
	}

	// Flush remaining rows.
	if len(buf) > 0 {
		if _, err := writer.WriteRows(buf); err != nil {
			return fmt.Errorf("write final rows: %w", err)
		}
	}

	return writer.Close()
}

// parquetValue converts a pgx-scanned value into a parquet value for the optional
// leaf column at index col. A SQL NULL, and a value the column cannot represent
// (NaN in a decimal column), gets definition level 0.
func parquetValue(v any, c column, col int) parquet.Value {
	var pv parquet.Value
	if c.typ == colDecimal {
		b := decimalBytes(v, c.scale, c.size)
		if b == nil {
			return parquet.NullValue().Level(0, 0, col)
		}
		pv = parquet.FixedLenByteArrayValue(b)
	} else {
		cv := convertValue(v, c.typ)
		if cv == nil {
			return parquet.NullValue().Level(0, 0, col)
		}
		pv = parquet.ValueOf(cv)
	}
	return pv.Level(0, 1, col)
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
	colDecimal   // numeric(p,s) → big-endian two's complement unscaled value
)

// column describes how one PG column is written to Parquet.
type column struct {
	typ   colType
	scale int // colDecimal: digits after the decimal point
	size  int // colDecimal: bytes of the fixed-length unscaled value
}

// maxDecimalPrecision is the widest DECIMAL that common Parquet readers
// (DuckDB, Arrow decimal128) accept.
const maxDecimalPrecision = 38

// numericTypmod decodes the precision and scale of a numeric(p,s) column from its
// type modifier; ok is false for an unconstrained numeric (typmod -1).
// See numeric_typmod_precision/scale in postgres src/backend/utils/adt/numeric.c.
func numericTypmod(typmod int32) (precision, scale int, ok bool) {
	if typmod < 4 { // VARHDRSZ
		return 0, 0, false
	}
	t := typmod - 4
	precision = int((t >> 16) & 0xffff)
	scale = int(((t & 0x7ff) ^ 1024) - 1024) // 11-bit signed since PG 15
	return precision, scale, true
}

// decimalSize is the number of bytes a two's complement integer needs to hold
// any value of the given decimal precision.
func decimalSize(precision int) int {
	limit := new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(precision)), nil)
	n := 1
	for new(big.Int).Lsh(big.NewInt(1), uint(8*n-1)).Cmp(limit) < 0 {
		n++
	}
	return n
}

// buildParquetSchema maps PG field descriptors to a parquet schema whose columns
// are in the same order as the query's columns.
func buildParquetSchema(fds []pgconn.FieldDescription) (*parquet.Schema, []column) {
	cols := make([]column, len(fds))
	group := columnGroup{
		Group:  make(parquet.Group, len(fds)),
		fields: make([]parquet.Field, len(fds)),
	}

	for i, fd := range fds {
		var node parquet.Node

		switch fd.DataTypeOID {
		case 16: // bool
			cols[i].typ = colBool
			node = parquet.Optional(parquet.Leaf(parquet.BooleanType))
		case 21: // int2
			cols[i].typ = colInt32
			node = parquet.Optional(parquet.Leaf(parquet.Int32Type))
		case 23: // int4
			cols[i].typ = colInt32
			node = parquet.Optional(parquet.Leaf(parquet.Int32Type))
		case 20: // int8
			cols[i].typ = colInt64
			node = parquet.Optional(parquet.Leaf(parquet.Int64Type))
		case 700: // float4
			cols[i].typ = colFloat32
			node = parquet.Optional(parquet.Leaf(parquet.FloatType))
		case 701: // float8
			cols[i].typ = colFloat64
			node = parquet.Optional(parquet.Leaf(parquet.DoubleType))
		case 1082: // date
			cols[i].typ = colDate
			node = parquet.Optional(parquet.Date())
		case 1114, 1184: // timestamp, timestamptz
			cols[i].typ = colTimestamp
			node = parquet.Optional(parquet.Timestamp(parquet.Microsecond))
		case 1700: // numeric
			// numeric(p,s) is written exactly as DECIMAL; unconstrained numeric (what
			// sum/avg and most WRDS columns give) and typmods DECIMAL cannot express
			// fall back to DOUBLE.
			if p, s, ok := numericTypmod(fd.TypeModifier); ok && p <= maxDecimalPrecision && s >= 0 && s <= p {
				size := decimalSize(p)
				cols[i] = column{typ: colDecimal, scale: s, size: size}
				node = parquet.Optional(parquet.Decimal(s, p, parquet.FixedLenByteArrayType(size)))
			} else {
				cols[i].typ = colFloat64
				node = parquet.Optional(parquet.Leaf(parquet.DoubleType))
			}
		default:
			// text (25), varchar (1043), char (18, 1042), etc.
			cols[i].typ = colString
			node = parquet.Optional(parquet.String())
		}

		group.Group[fd.Name] = node
		group.fields[i] = columnField{Node: node, name: fd.Name}
	}

	return parquet.NewSchema("wrds", group), cols
}

// columnGroup is a parquet group node that keeps its fields in insertion order;
// parquet.Group is a map and sorts its fields by name.
type columnGroup struct {
	parquet.Group
	fields []parquet.Field
}

func (g columnGroup) Fields() []parquet.Field { return g.fields }

// columnField is a named field of a columnGroup.
type columnField struct {
	parquet.Node
	name string
}

func (f columnField) Name() string { return f.name }

func (f columnField) Value(base reflect.Value) reflect.Value {
	if base.Kind() != reflect.Map {
		return reflect.Value{}
	}
	return base.MapIndex(reflect.ValueOf(f.name))
}

// decimalBytes returns the unscaled value of a numeric at the given scale as a
// size-byte big-endian two's complement integer, or nil for a value DECIMAL cannot
// hold (NULL, NaN).
func decimalBytes(v any, scale, size int) []byte {
	n, ok := v.(pgtype.Numeric)
	if !ok || !n.Valid || n.NaN || n.InfinityModifier != pgtype.Finite {
		return nil
	}
	u := new(big.Int)
	if n.Int != nil {
		u.Set(n.Int)
	}
	// Postgres stores values of a numeric(p,s) column at scale s, so the shift is
	// never negative in practice; truncate if it is.
	if shift := int64(n.Exp) + int64(scale); shift > 0 {
		u.Mul(u, new(big.Int).Exp(big.NewInt(10), big.NewInt(shift), nil))
	} else if shift < 0 {
		u.Quo(u, new(big.Int).Exp(big.NewInt(10), big.NewInt(-shift), nil))
	}
	if u.Sign() < 0 {
		u.Add(u, new(big.Int).Lsh(big.NewInt(1), uint(8*size)))
	}
	return u.FillBytes(make([]byte, size))
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
		switch f := v.(type) {
		case float64:
			return f
		case float32:
			return float64(f)
		case pgtype.Numeric:
			// NaN and ±Infinity map to the float values.
			if fv, err := f.Float64Value(); err == nil {
				if !fv.Valid {
					return nil
				}
				return fv.Float64
			}
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
