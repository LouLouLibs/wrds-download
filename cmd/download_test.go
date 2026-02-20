package cmd

import "testing"

func TestBuildQuery(t *testing.T) {
	tests := []struct {
		name    string
		setup   func()
		want    string
		wantErr bool
	}{
		{
			name: "raw query passthrough",
			setup: func() {
				dlQuery = "SELECT * FROM crsp.dsf LIMIT 10"
				dlSchema = ""
				dlTable = ""
			},
			want: "SELECT * FROM crsp.dsf LIMIT 10",
		},
		{
			name: "schema and table",
			setup: func() {
				dlQuery = ""
				dlSchema = "crsp"
				dlTable = "dsf"
				dlColumns = "*"
				dlWhere = ""
				dlLimit = 0
			},
			want: `SELECT * FROM "crsp"."dsf"`,
		},
		{
			name: "with columns",
			setup: func() {
				dlQuery = ""
				dlSchema = "comp"
				dlTable = "funda"
				dlColumns = "gvkey,datadate,sale"
				dlWhere = ""
				dlLimit = 0
			},
			want: `SELECT "gvkey", "datadate", "sale" FROM "comp"."funda"`,
		},
		{
			name: "with where and limit",
			setup: func() {
				dlQuery = ""
				dlSchema = "crsp"
				dlTable = "dsf"
				dlColumns = "*"
				dlWhere = "date >= '2020-01-01'"
				dlLimit = 1000
			},
			want: `SELECT * FROM "crsp"."dsf" WHERE date >= '2020-01-01' LIMIT 1000`,
		},
		{
			name: "missing schema and table",
			setup: func() {
				dlQuery = ""
				dlSchema = ""
				dlTable = ""
			},
			wantErr: true,
		},
		{
			name: "column with spaces trimmed",
			setup: func() {
				dlQuery = ""
				dlSchema = "crsp"
				dlTable = "dsf"
				dlColumns = " permno , date , prc "
				dlWhere = ""
				dlLimit = 0
			},
			want: `SELECT "permno", "date", "prc" FROM "crsp"."dsf"`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.setup()
			got, err := buildQuery()
			if (err != nil) != tt.wantErr {
				t.Fatalf("buildQuery() error = %v, wantErr %v", err, tt.wantErr)
			}
			if !tt.wantErr && got != tt.want {
				t.Errorf("buildQuery() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestResolveFormat(t *testing.T) {
	tests := []struct {
		path string
		flag string
		want string
	}{
		{"out.parquet", "", "parquet"},
		{"out.csv", "", "csv"},
		{"out.CSV", "", "csv"},
		{"out.parquet", "csv", "csv"},
		{"out.csv", "parquet", "parquet"},
		{"out.txt", "", "parquet"},
		{"out", "CSV", "csv"},
	}

	for _, tt := range tests {
		t.Run(tt.path+"_"+tt.flag, func(t *testing.T) {
			got := resolveFormat(tt.path, tt.flag)
			if got != tt.want {
				t.Errorf("resolveFormat(%q, %q) = %q, want %q", tt.path, tt.flag, got, tt.want)
			}
		})
	}
}
