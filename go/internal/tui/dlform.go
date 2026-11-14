package tui

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type dlFormField int

const (
	fieldSelect dlFormField = iota
	fieldWhere
	fieldLimit
	fieldOut
	fieldFormat
	fieldDecimal
	fieldCount
)

// DlForm is the download dialog overlay.
type DlForm struct {
	schema   string
	table    string
	database string // current database, for the CLI command
	inputs   [fieldCount]textinput.Model
	focused  dlFormField
	err      string
}

// DlSubmitMsg is sent when the user confirms the download form.
type DlSubmitMsg struct {
	Schema  string
	Table   string
	Columns string
	Where   string
	Limit   int
	Out     string
	Format  string
	Decimal bool
}

// DlCopyMsg is sent when the user asks for the CLI command instead of downloading.
type DlCopyMsg struct{ Command string }

// DlCancelMsg is sent when the user cancels.
type DlCancelMsg struct{}

// newDlForm opens the download dialog. selected pre-fills the SELECT field (all
// columns when empty); colNames is used for the placeholder hint.
func newDlForm(schema, table, database string, colNames, selected []string) DlForm {
	f := DlForm{schema: schema, table: table, database: database}

	f.inputs[fieldSelect] = textinput.New()
	placeholder := "e.g. gvkey, datadate, sale"
	if len(colNames) > 0 {
		hint := strings.Join(colNames, ", ")
		if len(hint) > 60 {
			hint = hint[:57] + "..."
		}
		placeholder = "e.g. " + hint
	}
	f.inputs[fieldSelect].Placeholder = placeholder
	f.inputs[fieldSelect].CharLimit = 1024
	f.inputs[fieldSelect].SetValue("*")
	if len(selected) > 0 {
		f.inputs[fieldSelect].SetValue(strings.Join(selected, ", "))
	}

	f.inputs[fieldWhere] = textinput.New()
	f.inputs[fieldWhere].Placeholder = "e.g. date >= '2020-01-01'"
	f.inputs[fieldWhere].CharLimit = 512

	f.inputs[fieldLimit] = textinput.New()
	f.inputs[fieldLimit].Placeholder = "no limit"
	f.inputs[fieldLimit].CharLimit = 12

	f.inputs[fieldOut] = textinput.New()
	f.inputs[fieldOut].Placeholder = fmt.Sprintf("./%s_%s.parquet", schema, table)
	f.inputs[fieldOut].CharLimit = 256
	f.inputs[fieldOut].SetValue(fmt.Sprintf("./%s_%s.parquet", schema, table))

	f.inputs[fieldFormat] = textinput.New()
	f.inputs[fieldFormat].Placeholder = "parquet"
	f.inputs[fieldFormat].CharLimit = 10
	f.inputs[fieldFormat].SetValue("parquet")

	f.inputs[fieldDecimal] = textinput.New()
	f.inputs[fieldDecimal].Placeholder = "n"
	f.inputs[fieldDecimal].CharLimit = 3
	f.inputs[fieldDecimal].SetValue("n")

	f.inputs[fieldSelect].Focus()
	return f
}

func (f DlForm) Update(msg tea.Msg) (DlForm, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "esc":
			return f, func() tea.Msg { return DlCancelMsg{} }
		case "enter":
			if f.focused < fieldCount-1 {
				f.inputs[f.focused].Blur()
				f.focused++
				f.inputs[f.focused].Focus()
				return f, textinput.Blink
			}
			m := f.submitMsg()
			return f, func() tea.Msg { return m }
		case "ctrl+y":
			c := cliCommand(f.submitMsg(), f.database)
			return f, func() tea.Msg { return DlCopyMsg{c} }
		case "tab", "down":
			f.inputs[f.focused].Blur()
			f.focused = (f.focused + 1) % fieldCount
			f.inputs[f.focused].Focus()
			return f, textinput.Blink
		case "shift+tab", "up":
			f.inputs[f.focused].Blur()
			f.focused = (f.focused + fieldCount - 1) % fieldCount
			f.inputs[f.focused].Focus()
			return f, textinput.Blink
		}
	}

	var cmd tea.Cmd
	f.inputs[f.focused], cmd = f.inputs[f.focused].Update(msg)
	return f, cmd
}

// submitMsg collects the form's values, with defaults for empty fields.
func (f DlForm) submitMsg() DlSubmitMsg {
	out := f.inputs[fieldOut].Value()
	if out == "" {
		out = fmt.Sprintf("./%s_%s.parquet", f.schema, f.table)
	}
	format := strings.ToLower(f.inputs[fieldFormat].Value())
	if format == "" {
		format = "parquet"
	}
	columns := strings.TrimSpace(f.inputs[fieldSelect].Value())
	if columns == "" {
		columns = "*"
	}
	var limit int
	if v := strings.TrimSpace(f.inputs[fieldLimit].Value()); v != "" {
		limit, _ = strconv.Atoi(v)
	}
	decimal := strings.HasPrefix(strings.ToLower(strings.TrimSpace(f.inputs[fieldDecimal].Value())), "y")
	return DlSubmitMsg{
		Schema:  f.schema,
		Table:   f.table,
		Columns: columns,
		Where:   f.inputs[fieldWhere].Value(),
		Limit:   limit,
		Out:     out,
		Format:  format,
		Decimal: decimal,
	}
}

func (f DlForm) View(width int) string {
	var sb strings.Builder

	title := stylePanelHeader.Render(fmt.Sprintf("Download  %s.%s", f.schema, f.table))
	sb.WriteString(title + "\n\n")

	labels := []string{"SELECT columns", "WHERE clause", "LIMIT rows", "Output path", "Format (parquet/csv)", "Exact decimals (y/n)"}
	for i, label := range labels {
		style := lipgloss.NewStyle().Foreground(colorMuted)
		if dlFormField(i) == f.focused {
			style = lipgloss.NewStyle().Foreground(colorFocus)
		}
		sb.WriteString(style.Render(label+"  ") + "\n")
		sb.WriteString(f.inputs[i].View() + "\n\n")
	}

	boxWidth := width - 8
	if boxWidth < 40 {
		boxWidth = 40
	}

	sb.WriteString(lipgloss.NewStyle().Foreground(colorMuted).Render("CLI command") + "\n")
	sb.WriteString(lipgloss.NewStyle().Width(boxWidth-6).Render(cliCommand(f.submitMsg(), f.database)) + "\n\n")

	hint := styleStatusBar.Render("[tab] next field   [enter] confirm   [ctrl+y] copy command   [esc] cancel")
	sb.WriteString(hint)

	content := sb.String()

	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(colorFocus).
		Padding(1, 2).
		Width(boxWidth).
		Render(content)

	return lipgloss.Place(width, lipgloss.Height(box), lipgloss.Center, lipgloss.Center, box)
}
