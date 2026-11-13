package export

import (
	"io"
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
