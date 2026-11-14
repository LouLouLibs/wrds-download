package export

import (
	"io"
	"math"
	"math/big"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/parquet-go/parquet-go"
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

func TestConvertValue(t *testing.T) {
	epoch := time.Date(1970, 1, 1, 0, 0, 0, 0, time.UTC)

	tests := []struct {
		name string
		v    any
		ct   colType
		want any
	}{
		{"nil", nil, colString, nil},
		{"bool true", true, colBool, true},
		{"bool false", false, colBool, false},
		{"int16 to int32", int16(42), colInt32, int32(42)},
		{"int32 passthrough", int32(100), colInt32, int32(100)},
		{"int64 to int32", int64(200), colInt32, int32(200)},
		{"int64 passthrough", int64(999), colInt64, int64(999)},
		{"int32 to int64", int32(50), colInt64, int64(50)},
		{"int16 to int64", int16(10), colInt64, int64(10)},
		{"float32 passthrough", float32(1.5), colFloat32, float32(1.5)},
		{"float64 to float32", float64(2.5), colFloat32, float32(2.5)},
		{"float64 passthrough", float64(3.14), colFloat64, float64(3.14)},
		{"float32 to float64", float32(1.5), colFloat64, float64(1.5)},
		{"date", time.Date(2020, 1, 2, 0, 0, 0, 0, time.UTC), colDate,
			int32(time.Date(2020, 1, 2, 0, 0, 0, 0, time.UTC).Sub(epoch).Hours() / 24)},
		{"timestamp", time.Date(2020, 6, 15, 12, 30, 0, 0, time.UTC), colTimestamp,
			time.Date(2020, 6, 15, 12, 30, 0, 0, time.UTC).Sub(epoch).Microseconds()},
		{"string passthrough", "hello", colString, "hello"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := convertValue(tt.v, tt.ct)
			if got != tt.want {
				t.Errorf("convertValue(%v, %v) = %v (%T), want %v (%T)", tt.v, tt.ct, got, got, tt.want, tt.want)
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
func (r *fakeRows) RawValues() [][]byte                          { return nil }
func (r *fakeRows) Conn() *pgx.Conn                              { return nil }

// TestWriteParquetZeroValues is a regression test for zero values (0, 0.0, false, "",
// the epoch) being written as NULL, and for columns being reordered by name.
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

	out := filepath.Join(t.TempDir(), "zero.parquet")
	if err := writeParquet(&fakeRows{fds: fds, data: data}, out, nil); err != nil {
		t.Fatalf("writeParquet: %v", err)
	}

	f, err := os.Open(out)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		t.Fatal(err)
	}
	pf, err := parquet.OpenFile(f, st.Size())
	if err != nil {
		t.Fatalf("open parquet: %v", err)
	}

	fields := pf.Schema().Fields()
	if len(fields) != len(fds) {
		t.Fatalf("got %d columns, want %d", len(fields), len(fds))
	}
	for i, fd := range fds {
		if fields[i].Name() != fd.Name {
			t.Errorf("column %d = %q, want %q", i, fields[i].Name(), fd.Name)
		}
	}

	reader := parquet.NewReader(pf)
	defer reader.Close()
	got := make([]parquet.Row, len(data))
	n, err := reader.ReadRows(got)
	if err != nil && err != io.EOF {
		t.Fatalf("read rows: %v", err)
	}
	if n != len(data) {
		t.Fatalf("read %d rows, want %d", n, len(data))
	}

	want := []any{int32(0), int64(0), float64(0), float32(0), "", false, int32(0), int64(0), int32(1), nil}
	for i, w := range want {
		v := got[0][i]
		if w == nil {
			if !v.IsNull() {
				t.Errorf("row 0 %s = %v, want NULL", fds[i].Name, v)
			}
			continue
		}
		if v.IsNull() {
			t.Errorf("row 0 %s is NULL, want %v", fds[i].Name, w)
			continue
		}
		if v.Kind() != parquet.ValueOf(w).Kind() || !parquet.Equal(v, parquet.ValueOf(w)) {
			t.Errorf("row 0 %s = %v, want %v", fds[i].Name, v, w)
		}
	}
	for i, v := range got[1] {
		if !v.IsNull() {
			t.Errorf("row 1 %s = %v, want NULL", fds[i].Name, v)
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

func TestDecimalSize(t *testing.T) {
	for p, want := range map[int]int{1: 1, 2: 1, 3: 2, 9: 4, 10: 5, 18: 8, 19: 9, 38: 16} {
		if got := decimalSize(p); got != want {
			t.Errorf("decimalSize(%d) = %d, want %d", p, got, want)
		}
	}
}

// TestWriteParquetNumeric is a regression test for numeric columns being written as
// strings: numeric(p,s) must come back as DECIMAL, unconstrained numeric as DOUBLE.
func TestWriteParquetNumeric(t *testing.T) {
	num := func(i int64, exp int32) pgtype.Numeric {
		return pgtype.Numeric{Int: big.NewInt(i), Exp: exp, Valid: true}
	}
	fds := []pgconn.FieldDescription{
		{Name: "n", DataTypeOID: 1700, TypeModifier: -1},
		{Name: "n_typed", DataTypeOID: 1700, TypeModifier: (10<<16 | 2) + 4},
		{Name: "n_wide", DataTypeOID: 1700, TypeModifier: (60<<16 | 2) + 4},
	}
	data := [][]any{
		{num(15, -1), num(1234, -2), num(15, -1)},
		{num(0, 0), num(-5, -1), num(0, 0)},
		{pgtype.Numeric{Valid: true, NaN: true}, pgtype.Numeric{Valid: true, NaN: true}, nil},
		{pgtype.Numeric{Valid: true, InfinityModifier: pgtype.NegativeInfinity}, nil, num(3, 2)},
	}

	out := filepath.Join(t.TempDir(), "numeric.parquet")
	if err := writeParquet(&fakeRows{fds: fds, data: data}, out, nil); err != nil {
		t.Fatalf("writeParquet: %v", err)
	}

	f, err := os.Open(out)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		t.Fatal(err)
	}
	pf, err := parquet.OpenFile(f, st.Size())
	if err != nil {
		t.Fatalf("open parquet: %v", err)
	}

	fields := pf.Schema().Fields()
	if k := fields[0].Type().Kind(); k != parquet.Double {
		t.Errorf("n kind = %v, want DOUBLE", k)
	}
	dec := fields[1].Type().LogicalType().Decimal
	if dec == nil || dec.Precision != 10 || dec.Scale != 2 {
		t.Errorf("n_typed logical type = %v, want DECIMAL(10,2)", fields[1].Type().LogicalType())
	}
	if k := fields[2].Type().Kind(); k != parquet.Double {
		t.Errorf("n_wide kind = %v, want DOUBLE (precision > 38)", k)
	}

	reader := parquet.NewReader(pf)
	defer reader.Close()
	got := make([]parquet.Row, len(data))
	n, err := reader.ReadRows(got)
	if err != nil && err != io.EOF {
		t.Fatalf("read rows: %v", err)
	}
	if n != len(data) {
		t.Fatalf("read %d rows, want %d", n, len(data))
	}

	// Unconstrained and too-wide numeric: float64, with NaN/Infinity kept.
	for r, want := range []float64{1.5, 0, math.NaN(), math.Inf(-1)} {
		v := got[r][0]
		if v.IsNull() || !(v.Double() == want || math.IsNaN(want) && math.IsNaN(v.Double())) {
			t.Errorf("row %d n = %v, want %v", r, v, want)
		}
	}
	if v := got[3][2]; v.IsNull() || v.Double() != 300 {
		t.Errorf("row 3 n_wide = %v, want 300", v)
	}
	if !got[2][2].IsNull() {
		t.Errorf("row 2 n_wide = %v, want NULL", got[2][2])
	}

	// numeric(10,2): unscaled big-endian two's complement in 5 bytes; NaN → NULL.
	for r, want := range map[int][]byte{
		0: {0, 0, 0, 0x04, 0xd2},          // 12.34 → 1234
		1: {0xff, 0xff, 0xff, 0xff, 0xce}, // -0.5 → -50
	} {
		if v := got[r][1]; v.IsNull() || string(v.ByteArray()) != string(want) {
			t.Errorf("row %d n_typed = %x, want %x", r, v.ByteArray(), want)
		}
	}
	for _, r := range []int{2, 3} {
		if !got[r][1].IsNull() {
			t.Errorf("row %d n_typed = %v, want NULL", r, got[r][1])
		}
	}
}
