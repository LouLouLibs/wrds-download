package export

import (
	"bytes"
	"fmt"
	"math"
	"math/big"
	"os"
	"time"

	"github.com/apache/arrow-go/v18/arrow"
	"github.com/apache/arrow-go/v18/arrow/array"
	"github.com/apache/arrow-go/v18/arrow/decimal128"
	"github.com/apache/arrow-go/v18/arrow/memory"
	"github.com/apache/arrow-go/v18/parquet"
	"github.com/apache/arrow-go/v18/parquet/compress"
	"github.com/apache/arrow-go/v18/parquet/pqarrow"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
)

// parquetResultFormats requests binary results for the types written as native
// Arrow types and text for every other column, which is then written as a string
// exactly as Postgres prints it (text, varchar, json, uuid, interval, arrays, ...).
var parquetResultFormats = pgx.QueryResultFormatsByOID{
	pgtype.BoolOID:        pgtype.BinaryFormatCode,
	pgtype.Int2OID:        pgtype.BinaryFormatCode,
	pgtype.Int4OID:        pgtype.BinaryFormatCode,
	pgtype.Int8OID:        pgtype.BinaryFormatCode,
	pgtype.Float4OID:      pgtype.BinaryFormatCode,
	pgtype.Float8OID:      pgtype.BinaryFormatCode,
	pgtype.NumericOID:     pgtype.BinaryFormatCode,
	pgtype.DateOID:        pgtype.BinaryFormatCode,
	pgtype.TimeOID:        pgtype.BinaryFormatCode,
	pgtype.TimestampOID:   pgtype.BinaryFormatCode,
	pgtype.TimestamptzOID: pgtype.BinaryFormatCode,
}

// colKind tags how a PG column is converted into its Arrow array.
type colKind int

const (
	kindText   colKind = iota // Postgres text output → string
	kindBpchar                // char(n) text output without its padding → string
	kindBool
	kindInt32
	kindInt64
	kindFloat32
	kindFloat64
	kindNumericFloat // unconstrained numeric → float64
	kindDecimal      // numeric(p,s) → decimal128
	kindDate
	kindTime
	kindTimestamp
)

// column describes how one PG column is written to Parquet.
type column struct {
	name  string
	kind  colKind
	scale int // kindDecimal: digits after the decimal point
}

// maxDecimalPrecision is the widest numeric(p,s) written as DECIMAL. Parquet2.jl
// reads DECIMAL into DecFP.Dec64, which holds 16 significant digits: a wider value
// makes the whole column fail to load. Wider numeric(p,s) is written as DOUBLE.
const maxDecimalPrecision = 16

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

// arrowColumn maps a PG field descriptor to its Arrow type.
func arrowColumn(fd pgconn.FieldDescription) (column, arrow.DataType) {
	c := column{name: fd.Name}
	switch fd.DataTypeOID {
	case pgtype.BoolOID:
		c.kind = kindBool
		return c, arrow.FixedWidthTypes.Boolean
	case pgtype.Int2OID, pgtype.Int4OID:
		c.kind = kindInt32
		return c, arrow.PrimitiveTypes.Int32
	case pgtype.Int8OID:
		c.kind = kindInt64
		return c, arrow.PrimitiveTypes.Int64
	case pgtype.Float4OID:
		c.kind = kindFloat32
		return c, arrow.PrimitiveTypes.Float32
	case pgtype.Float8OID:
		c.kind = kindFloat64
		return c, arrow.PrimitiveTypes.Float64
	case pgtype.NumericOID:
		// numeric(p,s) is written exactly as DECIMAL; unconstrained numeric (what
		// sum/avg and most WRDS columns give) and typmods DECIMAL cannot express
		// fall back to DOUBLE.
		if p, s, ok := numericTypmod(fd.TypeModifier); ok && p <= maxDecimalPrecision && s >= 0 && s <= p {
			c.kind, c.scale = kindDecimal, s
			return c, &arrow.Decimal128Type{Precision: int32(p), Scale: int32(s)}
		}
		c.kind = kindNumericFloat
		return c, arrow.PrimitiveTypes.Float64
	case pgtype.DateOID:
		c.kind = kindDate
		return c, arrow.FixedWidthTypes.Date32
	case pgtype.TimeOID:
		c.kind = kindTime
		return c, arrow.FixedWidthTypes.Time64us
	case pgtype.TimestampOID:
		c.kind = kindTimestamp
		return c, &arrow.TimestampType{Unit: arrow.Microsecond}
	case pgtype.TimestamptzOID:
		c.kind = kindTimestamp
		return c, &arrow.TimestampType{Unit: arrow.Microsecond, TimeZone: "UTC"}
	case pgtype.BPCharOID:
		// Trailing spaces of char(n) are insignificant in Postgres (length('AB'::char(4))
		// is 2); written padded, they break joins against text keys.
		c.kind = kindBpchar
		return c, arrow.BinaryTypes.String
	default:
		c.kind = kindText
		return c, arrow.BinaryTypes.String
	}
}

// writeParquet streams rows into a Parquet file through Arrow record batches, one
// row group per rowGroupSize rows. Columns that are not native Arrow types must have
// been requested in text format (parquetResultFormats). On error the output file is
// removed, so a failed export never leaves a valid-looking partial file behind.
func writeParquet(rows pgx.Rows, outPath string, progressFn func(int)) (err error) {
	fds := rows.FieldDescriptions()
	names := uniqueNames(fds)
	cols := make([]column, len(fds))
	fields := make([]arrow.Field, len(fds))
	for i, fd := range fds {
		var typ arrow.DataType
		cols[i], typ = arrowColumn(fd)
		cols[i].name = names[i]
		fields[i] = arrow.Field{Name: names[i], Type: typ, Nullable: true}
	}
	schema := arrow.NewSchema(fields, nil)

	f, err := os.Create(outPath)
	if err != nil {
		return fmt.Errorf("create parquet: %w", err)
	}
	defer f.Close()
	defer func() {
		if err != nil {
			os.Remove(outPath)
		}
	}()

	// DECIMAL is stored as INT32/INT64 rather than FIXED_LEN_BYTE_ARRAY: Parquet2.jl
	// does not sign-extend the byte-array form and reads negative values as huge
	// positive ones.
	props := parquet.NewWriterProperties(
		parquet.WithCompression(compress.Codecs.Zstd),
		parquet.WithStoreDecimalAsInteger(true),
	)
	writer, err := pqarrow.NewFileWriter(schema, f, props, pqarrow.DefaultWriterProps())
	if err != nil {
		return fmt.Errorf("parquet writer: %w", err)
	}
	defer writer.Close() // idempotent

	builder := array.NewRecordBuilder(memory.DefaultAllocator, schema)
	defer builder.Release()

	flush := func() error {
		rec := builder.NewRecordBatch()
		defer rec.Release()
		if err := writer.Write(rec); err != nil {
			return fmt.Errorf("write row group: %w", err)
		}
		return nil
	}

	total, pending := 0, 0
	for rows.Next() {
		vals, err := rows.Values()
		if err != nil {
			return fmt.Errorf("scan row: %w", err)
		}
		raw := rows.RawValues()
		for i, c := range cols {
			if err := appendValue(builder.Field(i), c, vals[i], raw[i]); err != nil {
				return fmt.Errorf("column %q: %w", c.name, err)
			}
		}
		pending++

		if pending == rowGroupSize {
			if err := flush(); err != nil {
				return err
			}
			total += pending
			pending = 0
			if progressFn != nil {
				progressFn(total)
			}
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("rows: %w", err)
	}

	if pending > 0 {
		if err := flush(); err != nil {
			return err
		}
	}
	return writer.Close()
}

// uniqueNames returns the column names with repeats (SELECT a.id, b.id ...) renamed
// to name_1, name_2, ..., skipping suffixes another column already uses, as DuckDB
// does. Parquet2.jl silently drops all but one of same-named columns.
func uniqueNames(fds []pgconn.FieldDescription) []string {
	taken := make(map[string]bool, len(fds))
	for _, fd := range fds {
		taken[fd.Name] = true
	}
	seen := make(map[string]bool, len(fds))
	names := make([]string, len(fds))
	for i, fd := range fds {
		name := fd.Name
		if seen[name] {
			for k := 1; ; k++ {
				if cand := fmt.Sprintf("%s_%d", fd.Name, k); !taken[cand] {
					name = cand
					break
				}
			}
			taken[name] = true
		}
		seen[name] = true
		names[i] = name
	}
	return names
}

// Postgres ±infinity dates and timestamps are written as the extreme values DuckDB
// uses for them, so they still sort after (before) every finite value.
const (
	dateInfinity      = math.MaxInt32
	timestampInfinity = math.MaxInt64
)

// appendValue appends one pgx value (v, or the raw text bytes for kindText) to b.
// A value that does not match the column's type is an error rather than being
// written as something else.
func appendValue(b array.Builder, c column, v any, raw []byte) error {
	if c.kind == kindText || c.kind == kindBpchar {
		if raw == nil {
			b.AppendNull()
			return nil
		}
		if c.kind == kindBpchar {
			raw = bytes.TrimRight(raw, " ")
		}
		b.(*array.StringBuilder).BinaryBuilder.Append(raw)
		return nil
	}
	if v == nil {
		b.AppendNull()
		return nil
	}

	switch c.kind {
	case kindBool:
		if x, ok := v.(bool); ok {
			b.(*array.BooleanBuilder).Append(x)
			return nil
		}
	case kindInt32:
		switch x := v.(type) {
		case int16:
			b.(*array.Int32Builder).Append(int32(x))
			return nil
		case int32:
			b.(*array.Int32Builder).Append(x)
			return nil
		}
	case kindInt64:
		if x, ok := v.(int64); ok {
			b.(*array.Int64Builder).Append(x)
			return nil
		}
	case kindFloat32:
		if x, ok := v.(float32); ok {
			b.(*array.Float32Builder).Append(x)
			return nil
		}
	case kindFloat64:
		if x, ok := v.(float64); ok {
			b.(*array.Float64Builder).Append(x)
			return nil
		}
	case kindNumericFloat:
		if x, ok := v.(pgtype.Numeric); ok {
			// NaN and ±Infinity map to the float values.
			f, err := x.Float64Value()
			if err != nil {
				return err
			}
			if !f.Valid {
				b.AppendNull()
			} else {
				b.(*array.Float64Builder).Append(f.Float64)
			}
			return nil
		}
	case kindDecimal:
		if x, ok := v.(pgtype.Numeric); ok {
			// DECIMAL cannot hold NaN (numeric(p,s) cannot hold ±Infinity).
			if !x.Valid || x.NaN || x.InfinityModifier != pgtype.Finite {
				b.AppendNull()
				return nil
			}
			b.(*array.Decimal128Builder).Append(decimal128.FromBigInt(unscaled(x, c.scale)))
			return nil
		}
	case kindDate:
		switch x := v.(type) {
		case time.Time:
			b.(*array.Date32Builder).Append(arrow.Date32(floorDiv(x.Unix(), 86400)))
			return nil
		case pgtype.InfinityModifier:
			b.(*array.Date32Builder).Append(arrow.Date32(int32(x) * dateInfinity))
			return nil
		}
	case kindTime:
		if x, ok := v.(pgtype.Time); ok {
			b.(*array.Time64Builder).Append(arrow.Time64(x.Microseconds))
			return nil
		}
	case kindTimestamp:
		switch x := v.(type) {
		case time.Time:
			b.(*array.TimestampBuilder).Append(arrow.Timestamp(x.UnixMicro()))
			return nil
		case pgtype.InfinityModifier:
			b.(*array.TimestampBuilder).Append(arrow.Timestamp(int64(x) * timestampInfinity))
			return nil
		}
	}
	return fmt.Errorf("unexpected value %v (%T)", v, v)
}

// unscaled returns the integer n × 10^scale of a finite numeric. Postgres stores
// values of a numeric(p,s) column at scale s, so no digits are dropped in practice;
// if the value has more, it is truncated.
func unscaled(n pgtype.Numeric, scale int) *big.Int {
	u := new(big.Int)
	if n.Int != nil {
		u.Set(n.Int)
	}
	if shift := int64(n.Exp) + int64(scale); shift > 0 {
		u.Mul(u, new(big.Int).Exp(big.NewInt(10), big.NewInt(shift), nil))
	} else if shift < 0 {
		u.Quo(u, new(big.Int).Exp(big.NewInt(10), big.NewInt(-shift), nil))
	}
	return u
}

func floorDiv(a, b int64) int64 {
	q := a / b
	if a%b != 0 && (a < 0) != (b < 0) {
		q--
	}
	return q
}
