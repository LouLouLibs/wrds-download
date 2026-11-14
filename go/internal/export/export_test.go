package export

import (
	"context"
	"fmt"
	"math"
	"math/big"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/apache/arrow-go/v18/arrow"
	"github.com/apache/arrow-go/v18/arrow/array"
	"github.com/apache/arrow-go/v18/arrow/memory"
	"github.com/apache/arrow-go/v18/parquet/pqarrow"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
)

func TestFormatValue(t *testing.T) {
	tests := []struct {
		name string
		v    any
		want string
	}{
		{"nil", nil, ""},
		{"string", "hello", "hello"},
		{"bytes", []byte("data"), "data"},
		{"int", 42, "42"},
		{"float", 3.14, "3.14"},
		{"bool", true, "true"},
		{"date only", time.Date(2020, 1, 2, 0, 0, 0, 0, time.UTC), "2020-01-02"},
		{"datetime", time.Date(2020, 1, 2, 15, 4, 5, 0, time.UTC), "2020-01-02T15:04:05Z"},
		{"numeric valid", pgtype.Numeric{Int: big.NewInt(12345), Exp: -2, Valid: true}, "123.45"},
		{"numeric invalid", pgtype.Numeric{Valid: false}, ""},
		{"numeric NaN", pgtype.Numeric{Valid: true, NaN: true}, "NaN"},
		{"numeric Infinity", pgtype.Numeric{Valid: true, InfinityModifier: pgtype.Infinity}, "Infinity"},
		{"numeric -Infinity", pgtype.Numeric{Valid: true, InfinityModifier: pgtype.NegativeInfinity}, "-Infinity"},
		{"numeric zero exp", pgtype.Numeric{Int: big.NewInt(42), Exp: 0, Valid: true}, "42"},
		{"numeric positive exp", pgtype.Numeric{Int: big.NewInt(5), Exp: 3, Valid: true}, "5000"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatValue(tt.v)
			if got != tt.want {
				t.Errorf("formatValue(%v) = %q, want %q", tt.v, got, tt.want)
			}
		})
	}
}

// fakeRows is an in-memory pgx.Rows for exercising the writers without a database.
type fakeRows struct {
	fds  []pgconn.FieldDescription
	data [][]any
	i    int
}

func (r *fakeRows) Close()                                       {}
func (r *fakeRows) Err() error                                   { return nil }
func (r *fakeRows) CommandTag() pgconn.CommandTag                { return pgconn.CommandTag{} }
func (r *fakeRows) FieldDescriptions() []pgconn.FieldDescription { return r.fds }
func (r *fakeRows) Next() bool                                   { r.i++; return r.i <= len(r.data) }
func (r *fakeRows) Scan(dest ...any) error                       { return nil }
func (r *fakeRows) Values() ([]any, error)                       { return r.data[r.i-1], nil }
func (r *fakeRows) RawValues() [][]byte {
	raw := make([][]byte, len(r.fds))
	for i, v := range r.data[r.i-1] {
		if s, ok := v.(string); ok {
			raw[i] = []byte(s)
		}
	}
	return raw
}
func (r *fakeRows) Conn() *pgx.Conn { return nil }

// readParquet reads a file written by writeParquet back as an Arrow table.
func readParquet(t *testing.T, path string) arrow.Table {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { f.Close() })
	tbl, err := pqarrow.ReadTable(context.Background(), f, nil, pqarrow.ArrowReadProperties{}, memory.DefaultAllocator)
	if err != nil {
		t.Fatalf("read parquet: %v", err)
	}
	t.Cleanup(tbl.Release)
	return tbl
}

// exportParquet writes data through writeParquet and reads it back.
func exportParquet(t *testing.T, fds []pgconn.FieldDescription, data [][]any) arrow.Table {
	t.Helper()
	out := filepath.Join(t.TempDir(), "out.parquet")
	if err := writeParquet(&fakeRows{fds: fds, data: data}, out, nil); err != nil {
		t.Fatalf("writeParquet: %v", err)
	}
	tbl := readParquet(t, out)
	if int(tbl.NumRows()) != len(data) {
		t.Fatalf("read %d rows, want %d", tbl.NumRows(), len(data))
	}
	return tbl
}

// cell returns row r of column c as a single-chunk array and index.
func cell(t *testing.T, tbl arrow.Table, c, r int) (arrow.Array, int) {
	t.Helper()
	chunks := tbl.Column(c).Data().Chunks()
	for _, ch := range chunks {
		if r < ch.Len() {
			return ch, r
		}
		r -= ch.Len()
	}
	t.Fatalf("row out of range")
	return nil, 0
}

// TestWriteParquetZeroValues is a regression test for zero values (0, 0.0, false, "",
// the epoch) being written as NULL (#17), and for columns being reordered by name.
func TestWriteParquetZeroValues(t *testing.T) {
	epoch := time.Date(1970, 1, 1, 0, 0, 0, 0, time.UTC)
	fds := []pgconn.FieldDescription{
		{Name: "i4", DataTypeOID: 23},
		{Name: "i8", DataTypeOID: 20},
		{Name: "f8", DataTypeOID: 701},
		{Name: "f4", DataTypeOID: 700},
		{Name: "s", DataTypeOID: 25},
		{Name: "b", DataTypeOID: 16},
		{Name: "d", DataTypeOID: 1082},
		{Name: "ts", DataTypeOID: 1114},
		{Name: "one", DataTypeOID: 23},
		{Name: "true_null", DataTypeOID: 23},
	}
	data := [][]any{
		{int32(0), int64(0), float64(0), float32(0), "", false, epoch, epoch, int32(1), nil},
		{nil, nil, nil, nil, nil, nil, nil, nil, nil, nil},
	}
	tbl := exportParquet(t, fds, data)

	for i, fd := range fds {
		if got := tbl.Schema().Field(i).Name; got != fd.Name {
			t.Errorf("column %d = %q, want %q", i, got, fd.Name)
		}
	}
	want := []string{"0", "0", "0", "0", `""`, "false", "1970-01-01", "1970-01-01T00:00:00Z", "1", "(null)"}
	for i, w := range want {
		a, r := cell(t, tbl, i, 0)
		got := a.ValueStr(r)
		if a.IsNull(r) {
			got = "(null)"
		} else if fds[i].Name == "s" {
			got = `"` + got + `"`
		} else if fds[i].Name == "ts" {
			got = a.(*array.Timestamp).Value(r).ToTime(arrow.Microsecond).Format(time.RFC3339)
		}
		if got != w {
			t.Errorf("row 0 %s = %s, want %s", fds[i].Name, got, w)
		}
	}
	for i := range fds {
		if a, r := cell(t, tbl, i, 1); !a.IsNull(r) {
			t.Errorf("row 1 %s = %s, want NULL", fds[i].Name, a.ValueStr(r))
		}
	}
}

func TestNumericTypmod(t *testing.T) {
	tests := []struct {
		name         string
		typmod       int32
		wantP, wantS int
		wantOK       bool
	}{
		{"unconstrained", -1, 0, 0, false},
		{"numeric(10,2)", (10<<16 | 2) + 4, 10, 2, true},
		{"numeric(38,0)", (38 << 16) + 4, 38, 0, true},
		{"numeric(2,-3)", (2<<16 | (-3 & 0x7ff)) + 4, 2, -3, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p, s, ok := numericTypmod(tt.typmod)
			if p != tt.wantP || s != tt.wantS || ok != tt.wantOK {
				t.Errorf("numericTypmod(%d) = (%d, %d, %v), want (%d, %d, %v)",
					tt.typmod, p, s, ok, tt.wantP, tt.wantS, tt.wantOK)
			}
		})
	}
}

// TestWriteParquetNumeric is a regression test for numeric columns being written as
// strings (#19): numeric(p,s) up to p = 16 must come back as DECIMAL, unconstrained and
// wider numeric as DOUBLE.
func TestWriteParquetNumeric(t *testing.T) {
	num := func(i int64, exp int32) pgtype.Numeric {
		return pgtype.Numeric{Int: big.NewInt(i), Exp: exp, Valid: true}
	}
	big11, _ := new(big.Int).SetString("12345678901", 10)
	fds := []pgconn.FieldDescription{
		{Name: "n", DataTypeOID: 1700, TypeModifier: -1},
		{Name: "n_typed", DataTypeOID: 1700, TypeModifier: (10<<16 | 2) + 4},
		{Name: "n_wide", DataTypeOID: 1700, TypeModifier: (60<<16 | 2) + 4},
		{Name: "n_16", DataTypeOID: 1700, TypeModifier: (16<<16 | 6) + 4},
		{Name: "n_17", DataTypeOID: 1700, TypeModifier: (17<<16 | 6) + 4},
	}
	data := [][]any{
		{num(15, -1), num(1234, -2), num(15, -1), num(-9999999999999999, -6), pgtype.Numeric{Int: big11, Exp: -1, Valid: true}},
		{num(0, 0), num(-5, -1), num(0, 0), num(0, 0), num(5, -1)},
		{pgtype.Numeric{Valid: true, NaN: true}, pgtype.Numeric{Valid: true, NaN: true}, nil, nil, nil},
		{pgtype.Numeric{Valid: true, InfinityModifier: pgtype.NegativeInfinity}, nil, num(3, 2), num(1, 0), num(1, 0)},
	}
	tbl := exportParquet(t, fds, data)

	wantTypes := []string{"float64", "decimal(10, 2)", "float64", "decimal(16, 6)", "float64"}
	for i, w := range wantTypes {
		if got := tbl.Schema().Field(i).Type.String(); got != w {
			t.Errorf("%s type = %s, want %s", fds[i].Name, got, w)
		}
	}

	want := [][]string{
		{"1.5", "12.34", "1.5", "-9999999999.999999", "1.2345678901e+09"},
		{"0", "-0.50", "0", "0.000000", "0.5"},
		{"NaN", "(null)", "(null)", "(null)", "(null)"},
		{"-Inf", "(null)", "300", "1.000000", "1"},
	}
	for r, row := range want {
		for c, w := range row {
			a, i := cell(t, tbl, c, r)
			var got string
			switch {
			case a.IsNull(i):
				got = "(null)"
			case a.DataType().ID() == arrow.DECIMAL128:
				dt := a.DataType().(*arrow.Decimal128Type)
				got = a.(*array.Decimal128).Value(i).ToString(dt.Scale)
			default:
				got = a.ValueStr(i)
			}
			if got != w {
				t.Errorf("row %d %s = %s, want %s", r, fds[c].Name, got, w)
			}
		}
	}
}

// TestWriteParquetTemporal covers dates and timestamps far from 1970 (WRDS uses
// 9999-12-31 as an open end date), ±infinity, time of day, and whether timestamps
// are marked as UTC-adjusted.
func TestWriteParquetTemporal(t *testing.T) {
	day := func(y int, m time.Month, d int) time.Time { return time.Date(y, m, d, 0, 0, 0, 0, time.UTC) }
	fds := []pgconn.FieldDescription{
		{Name: "d", DataTypeOID: 1082},
		{Name: "ts", DataTypeOID: 1114},
		{Name: "tstz", DataTypeOID: 1184},
		{Name: "tm", DataTypeOID: 1083},
	}
	ny := time.FixedZone("EST", -5*3600)
	data := [][]any{
		{day(9999, 12, 31), time.Date(9999, 12, 31, 23, 59, 59, 999999000, time.UTC),
			time.Date(2020, 1, 2, 10, 0, 0, 0, ny), pgtype.Time{Microseconds: 34200_000000, Valid: true}},
		{day(1, 1, 1), day(1, 1, 1), day(1960, 6, 30), pgtype.Time{Microseconds: 0, Valid: true}},
		{pgtype.Infinity, pgtype.Infinity, pgtype.NegativeInfinity, nil},
	}
	tbl := exportParquet(t, fds, data)

	if tz := tbl.Schema().Field(1).Type.(*arrow.TimestampType).TimeZone; tz != "" {
		t.Errorf("timestamp time zone = %q, want none", tz)
	}
	if tz := tbl.Schema().Field(2).Type.(*arrow.TimestampType).TimeZone; tz != "UTC" {
		t.Errorf("timestamptz time zone = %q, want UTC", tz)
	}

	dateStr := func(r int) string {
		a, i := cell(t, tbl, 0, r)
		return a.(*array.Date32).Value(i).ToTime().Format("2006-01-02")
	}
	tsStr := func(c, r int) string {
		a, i := cell(t, tbl, c, r)
		return a.(*array.Timestamp).Value(i).ToTime(arrow.Microsecond).Format("2006-01-02T15:04:05.999999Z")
	}
	tm := func(r int) arrow.Time64 {
		a, i := cell(t, tbl, 3, r)
		return a.(*array.Time64).Value(i)
	}

	checks := []struct{ name, got, want string }{
		{"d 9999-12-31", dateStr(0), "9999-12-31"},
		{"d 0001-01-01", dateStr(1), "0001-01-01"},
		{"ts 9999", tsStr(1, 0), "9999-12-31T23:59:59.999999Z"},
		{"ts 0001", tsStr(1, 1), "0001-01-01T00:00:00Z"},
		{"tstz EST → UTC", tsStr(2, 0), "2020-01-02T15:00:00Z"},
		{"tstz pre-1970", tsStr(2, 1), "1960-06-30T00:00:00Z"},
	}
	for _, c := range checks {
		if c.got != c.want {
			t.Errorf("%s = %s, want %s", c.name, c.got, c.want)
		}
	}
	if got := tm(0); got != 34200_000000 {
		t.Errorf("tm 09:30 = %d µs, want 34200000000", got)
	}
	if a, i := cell(t, tbl, 0, 2); a.(*array.Date32).Value(i) != math.MaxInt32 {
		t.Errorf("d infinity = %d, want MaxInt32", a.(*array.Date32).Value(i))
	}
	if a, i := cell(t, tbl, 1, 2); a.(*array.Timestamp).Value(i) != math.MaxInt64 {
		t.Errorf("ts infinity = %d, want MaxInt64", a.(*array.Timestamp).Value(i))
	}
	if a, i := cell(t, tbl, 2, 2); a.(*array.Timestamp).Value(i) != -math.MaxInt64 {
		t.Errorf("tstz -infinity = %d, want -MaxInt64", a.(*array.Timestamp).Value(i))
	}
	if a, i := cell(t, tbl, 3, 2); !a.IsNull(i) {
		t.Errorf("tm NULL = %s, want NULL", a.ValueStr(i))
	}
}

// TestWriteParquetText checks that types without a native mapping are written as
// their Postgres text output.
func TestWriteParquetText(t *testing.T) {
	fds := []pgconn.FieldDescription{
		{Name: "j", DataTypeOID: 3802},  // jsonb
		{Name: "u", DataTypeOID: 2950},  // uuid
		{Name: "a", DataTypeOID: 1007},  // int4[]
		{Name: "c", DataTypeOID: 1042},  // bpchar
		{Name: "iv", DataTypeOID: 1186}, // interval
	}
	data := [][]any{
		{`{"a": 1}`, "a0eebc99-9c0b-4ef8-bb6d-6bb9bd380a11", "{1,2,NULL}", "AB", "1 day 02:00:00"},
		{nil, nil, nil, nil, nil},
	}
	tbl := exportParquet(t, fds, data)
	for i, w := range data[0] {
		if typ := tbl.Schema().Field(i).Type; typ.ID() != arrow.STRING {
			t.Errorf("%s type = %s, want utf8", fds[i].Name, typ)
		}
		a, r := cell(t, tbl, i, 0)
		if got := a.(*array.String).Value(r); got != w {
			t.Errorf("%s = %q, want %q", fds[i].Name, got, w)
		}
		if a, r := cell(t, tbl, i, 1); !a.IsNull(r) {
			t.Errorf("%s NULL = %q, want NULL", fds[i].Name, a.ValueStr(r))
		}
	}
}

func TestWriteParquetEmpty(t *testing.T) {
	tbl := exportParquet(t, []pgconn.FieldDescription{{Name: "x", DataTypeOID: 23}}, nil)
	if tbl.NumCols() != 1 || tbl.Schema().Field(0).Type.ID() != arrow.INT32 {
		t.Errorf("schema = %s, want x: int32", tbl.Schema())
	}
}

// TestWriteParquetRowGroups checks the row group split and the progress callback.
func TestWriteParquetRowGroups(t *testing.T) {
	n := rowGroupSize*2 + 5
	data := make([][]any, n)
	for i := range data {
		data[i] = []any{int64(i)}
	}
	var progress []int
	out := filepath.Join(t.TempDir(), "rg.parquet")
	fds := []pgconn.FieldDescription{{Name: "i", DataTypeOID: 20}}
	if err := writeParquet(&fakeRows{fds: fds, data: data}, out, func(n int) { progress = append(progress, n) }); err != nil {
		t.Fatal(err)
	}
	if len(progress) != 2 || progress[0] != rowGroupSize || progress[1] != 2*rowGroupSize {
		t.Errorf("progress = %v", progress)
	}
	tbl := readParquet(t, out)
	if tbl.NumRows() != int64(n) {
		t.Fatalf("rows = %d, want %d", tbl.NumRows(), n)
	}
	if a, r := cell(t, tbl, 0, n-1); a.(*array.Int64).Value(r) != int64(n-1) {
		t.Errorf("last value = %d, want %d", a.(*array.Int64).Value(r), n-1)
	}
}

// TestWriteParquetTypeMismatch checks that a value of an unexpected Go type fails the
// export and leaves no output file, instead of being written as something else.
func TestWriteParquetTypeMismatch(t *testing.T) {
	out := filepath.Join(t.TempDir(), "bad.parquet")
	fds := []pgconn.FieldDescription{{Name: "d", DataTypeOID: 1082}}
	err := writeParquet(&fakeRows{fds: fds, data: [][]any{{"2020-01-01"}}}, out, nil)
	if err == nil {
		t.Fatal("writeParquet succeeded, want an error")
	}
	if _, statErr := os.Stat(out); !os.IsNotExist(statErr) {
		t.Errorf("output file left behind after error: %v", statErr)
	}
}

func TestUniqueNames(t *testing.T) {
	fd := func(names ...string) []pgconn.FieldDescription {
		fds := make([]pgconn.FieldDescription, len(names))
		for i, n := range names {
			fds[i].Name = n
		}
		return fds
	}
	tests := []struct {
		in, want []string
	}{
		{[]string{"a", "b"}, []string{"a", "b"}},
		{[]string{"id", "id"}, []string{"id", "id_1"}},
		{[]string{"id", "id", "id"}, []string{"id", "id_1", "id_2"}},
		// an existing id_1 keeps its name; the repeat skips to id_2
		{[]string{"id", "id", "id_1"}, []string{"id", "id_2", "id_1"}},
		{[]string{"x", "x_1", "x", "x_1"}, []string{"x", "x_1", "x_2", "x_1_1"}},
	}
	for _, tt := range tests {
		got := uniqueNames(fd(tt.in...))
		if fmt.Sprint(got) != fmt.Sprint(tt.want) {
			t.Errorf("uniqueNames(%v) = %v, want %v", tt.in, got, tt.want)
		}
	}
}

// TestWriteParquetNamesAndBpchar is a regression test for #23: repeated column names
// are deduplicated and char(n) values lose their padding.
func TestWriteParquetNamesAndBpchar(t *testing.T) {
	fds := []pgconn.FieldDescription{
		{Name: "id", DataTypeOID: 23},
		{Name: "id", DataTypeOID: 23},
		{Name: "ch", DataTypeOID: 1042},
		{Name: "vc", DataTypeOID: 1043},
	}
	data := [][]any{
		{int32(1), int32(2), "AB  ", "AB  "},
		{nil, nil, "    ", ""},
		{nil, nil, nil, nil},
	}
	tbl := exportParquet(t, fds, data)

	for i, w := range []string{"id", "id_1", "ch", "vc"} {
		if got := tbl.Schema().Field(i).Name; got != w {
			t.Errorf("column %d = %q, want %q", i, got, w)
		}
	}
	if a, r := cell(t, tbl, 1, 0); a.(*array.Int32).Value(r) != 2 {
		t.Errorf("id_1 = %d, want 2", a.(*array.Int32).Value(r))
	}
	// char(n) is trimmed (all-blank becomes ""); varchar keeps its spaces.
	for r, want := range [][2]string{{"AB", "AB  "}, {"", ""}} {
		for c := 2; c <= 3; c++ {
			a, i := cell(t, tbl, c, r)
			if got := a.(*array.String).Value(i); a.IsNull(i) || got != want[c-2] {
				t.Errorf("row %d %s = %q, want %q", r, fds[c].Name, got, want[c-2])
			}
		}
	}
	if a, i := cell(t, tbl, 2, 2); !a.IsNull(i) {
		t.Errorf("row 2 ch = %q, want NULL", a.ValueStr(i))
	}
}
