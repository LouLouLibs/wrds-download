package db

import "testing"

func TestQuoteIdent(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"crsp", `"crsp"`},
		{"dsf", `"dsf"`},
		{`my"table`, `"my""table"`},
		{"", `""`},
		{"with space", `"with space"`},
		{`double""quote`, `"double""""quote"`},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := QuoteIdent(tt.input)
			if got != tt.want {
				t.Errorf("QuoteIdent(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}
