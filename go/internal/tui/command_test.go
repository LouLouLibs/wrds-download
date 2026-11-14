package tui

import "testing"

func TestCLICommand(t *testing.T) {
	tests := []struct {
		name string
		msg  DlSubmitMsg
		db   string
		want string
	}{
		{
			name: "defaults",
			msg:  DlSubmitMsg{Schema: "crsp", Table: "msf", Columns: "*", Out: "./crsp_msf.parquet", Format: "parquet"},
			db:   "wrds",
			want: "wrds-dl download --schema crsp --table msf --out ./crsp_msf.parquet",
		},
		{
			name: "everything",
			msg: DlSubmitMsg{Schema: "crsp", Table: "msf", Columns: " permno, date ,ret", Where: "date >= '2020-01-01'",
				Limit: 1000, Out: "./my file.csv", Format: "csv", Decimal: true},
			db:   "wrds",
			want: `wrds-dl download --schema crsp --table msf --columns permno,date,ret --where "date >= '2020-01-01'" --limit 1000 --out "./my file.csv" --decimal`,
		},
		{
			name: "format differs from extension",
			msg:  DlSubmitMsg{Schema: "comp", Table: "funda", Out: "./out.dat", Format: "csv"},
			want: "wrds-dl download --schema comp --table funda --out ./out.dat --format csv",
		},
		{
			name: "other database",
			msg:  DlSubmitMsg{Schema: "s", Table: "t", Out: "t.parquet", Format: "parquet"},
			db:   "wrds_other",
			want: "PGDATABASE=wrds_other wrds-dl download --schema s --table t --out t.parquet",
		},
		{
			name: "quotes needing single quotes",
			msg:  DlSubmitMsg{Schema: "s", Table: "t", Columns: `"Odd Name"`, Where: `name = 'O''Brien' AND x != '$5'`, Out: "t.parquet"},
			want: `wrds-dl download --schema s --table t --columns '"Odd Name"' --where 'name = '\''O'\'''\''Brien'\'' AND x != '\''$5'\''' --out t.parquet`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := cliCommand(tt.msg, tt.db); got != tt.want {
				t.Errorf("got  %s\nwant %s", got, tt.want)
			}
		})
	}
}
