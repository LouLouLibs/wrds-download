package export

import (
	"math/big"
	"testing"
	"time"

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
