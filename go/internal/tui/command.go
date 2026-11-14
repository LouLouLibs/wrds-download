package tui

import (
	"fmt"
	"regexp"
	"strings"
)

// cliCommand returns the `wrds-dl download` command line that downloads what the
// form describes, for pasting into a shell or script. database is the TUI's current
// database; anything other than the default "wrds" is passed through PGDATABASE.
func cliCommand(m DlSubmitMsg, database string) string {
	var args []string
	if database != "" && database != "wrds" {
		args = append(args, "PGDATABASE="+shellQuote(database))
	}
	args = append(args, "wrds-dl", "download",
		"--schema", shellQuote(m.Schema),
		"--table", shellQuote(m.Table))
	if cols := normalizeColumns(m.Columns); cols != "*" {
		args = append(args, "--columns", shellQuote(cols))
	}
	if w := strings.TrimSpace(m.Where); w != "" {
		args = append(args, "--where", shellQuote(w))
	}
	if m.Limit > 0 {
		args = append(args, "--limit", fmt.Sprint(m.Limit))
	}
	args = append(args, "--out", shellQuote(m.Out))
	inferred := "parquet"
	if strings.HasSuffix(strings.ToLower(m.Out), ".csv") {
		inferred = "csv"
	}
	if m.Format != "" && m.Format != inferred {
		args = append(args, "--format", shellQuote(m.Format))
	}
	if m.Decimal {
		args = append(args, "--decimal")
	}
	return strings.Join(args, " ")
}

// normalizeColumns turns the form's "a, b ,c" into "a,b,c" ("*" when empty).
func normalizeColumns(s string) string {
	s = strings.TrimSpace(s)
	if s == "" || s == "*" {
		return "*"
	}
	parts := strings.Split(s, ",")
	for i, p := range parts {
		parts[i] = strings.TrimSpace(p)
	}
	return strings.Join(parts, ",")
}

var shellSafe = regexp.MustCompile(`^[A-Za-z0-9_@%+=:,./-]+$`)

// shellQuote quotes s for a POSIX shell: bare when safe, in double quotes when
// that needs no escaping (so WHERE clauses read naturally:
// "date >= '2020-01-01'"), and in single quotes otherwise.
func shellQuote(s string) string {
	if shellSafe.MatchString(s) {
		return s
	}
	if !strings.ContainsAny(s, "\"$`\\!") {
		return `"` + s + `"`
	}
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}
