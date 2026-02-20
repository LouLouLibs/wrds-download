package tui

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/table"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/eloualiche/wrds-download/internal/db"
	"github.com/eloualiche/wrds-download/internal/export"
)

// pane identifies which panel is focused.
type pane int

const (
	paneSchema pane = iota
	paneTable
	panePreview
)

// appState represents the TUI state machine.
type appState int

const (
	stateBrowse appState = iota
	stateDownloadForm
	stateDownloading
	stateDone
)

// -- Tea messages --

type schemasLoadedMsg struct{ schemas []db.Schema }
type tablesLoadedMsg struct{ tables []db.Table }
type previewLoadedMsg struct{ result *db.PreviewResult }
type errMsg struct{ err error }
type downloadDoneMsg struct{ path string }
type tickMsg time.Time

func errCmd(err error) tea.Cmd {
	return func() tea.Msg { return errMsg{err} }
}

// item wraps a string to satisfy the bubbles list.Item interface.
type item struct{ title string }

func (i item) FilterValue() string { return i.title }
func (i item) Title() string       { return i.title }
func (i item) Description() string { return "" }

// App is the root Bubble Tea model.
type App struct {
	client *db.Client

	width, height int
	focus         pane
	state         appState

	schemaList  list.Model
	tableList   list.Model
	previewTbl  table.Model
	previewInfo string // "~2.1M rows" etc.

	dlForm   DlForm
	spinner  spinner.Model
	statusOK string
	statusErr string

	selectedSchema string
	selectedTable  string
}

// NewApp constructs the root model.
func NewApp(client *db.Client) *App {
	del := list.NewDefaultDelegate()
	del.ShowDescription = false

	schemaList := list.New(nil, del, 0, 0)
	schemaList.Title = "Schemas"
	schemaList.SetShowStatusBar(false)
	schemaList.SetFilteringEnabled(true)

	tableList := list.New(nil, del, 0, 0)
	tableList.Title = "Tables"
	tableList.SetShowStatusBar(false)
	tableList.SetFilteringEnabled(true)

	sp := spinner.New()
	sp.Spinner = spinner.Dot

	return &App{
		client:     client,
		schemaList: schemaList,
		tableList:  tableList,
		spinner:    sp,
		focus:      paneSchema,
		state:      stateBrowse,
	}
}

// Init loads schemas on startup.
func (a *App) Init() tea.Cmd {
	return tea.Batch(
		a.loadSchemas(),
		a.spinner.Tick,
	)
}

func (a *App) loadSchemas() tea.Cmd {
	return func() tea.Msg {
		schemas, err := a.client.Schemas(context.Background())
		if err != nil {
			return errMsg{err}
		}
		return schemasLoadedMsg{schemas}
	}
}

func (a *App) loadTables(schema string) tea.Cmd {
	return func() tea.Msg {
		tables, err := a.client.Tables(context.Background(), schema)
		if err != nil {
			return errMsg{err}
		}
		return tablesLoadedMsg{tables}
	}
}

func (a *App) loadPreview(schema, tbl string) tea.Cmd {
	return func() tea.Msg {
		result, err := a.client.Preview(context.Background(), schema, tbl, 50)
		if err != nil {
			return errMsg{err}
		}
		return previewLoadedMsg{result}
	}
}

func (a *App) startDownload(msg DlSubmitMsg) tea.Cmd {
	return func() tea.Msg {
		var query string
		if msg.Where != "" {
			query = fmt.Sprintf("SELECT * FROM wrds.%s.%s WHERE %s", msg.Schema, msg.Table, msg.Where)
		} else {
			query = fmt.Sprintf("SELECT * FROM wrds.%s.%s", msg.Schema, msg.Table)
		}
		err := export.Export(query, msg.Out, export.Options{Format: msg.Format})
		if err != nil {
			return errMsg{err}
		}
		return downloadDoneMsg{msg.Out}
	}
}

// Update handles all messages.
func (a *App) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {

	case tea.WindowSizeMsg:
		a.width = msg.Width
		a.height = msg.Height
		a.resizePanels()
		return a, nil

	case spinner.TickMsg:
		var cmd tea.Cmd
		a.spinner, cmd = a.spinner.Update(msg)
		return a, cmd

	case schemasLoadedMsg:
		items := make([]list.Item, len(msg.schemas))
		for i, s := range msg.schemas {
			items[i] = item{s.Name}
		}
		a.schemaList.SetItems(items)
		return a, nil

	case tablesLoadedMsg:
		items := make([]list.Item, len(msg.tables))
		for i, t := range msg.tables {
			items[i] = item{t.Name}
		}
		a.tableList.SetItems(items)
		a.previewTbl = table.Model{} // clear preview
		a.previewInfo = ""
		return a, nil

	case previewLoadedMsg:
		r := msg.result
		cols := make([]table.Column, len(r.Columns))
		for i, c := range r.Columns {
			w := maxWidth(c, r.Rows, i, 20)
			cols[i] = table.Column{Title: c, Width: w}
		}
		rows := make([]table.Row, len(r.Rows))
		for i, row := range r.Rows {
			rows[i] = table.Row(row)
		}
		t := table.New(
			table.WithColumns(cols),
			table.WithRows(rows),
			table.WithFocused(false),
			table.WithHeight(a.previewHeight()-4),
		)
		ts := table.DefaultStyles()
		ts.Header = ts.Header.BorderStyle(lipgloss.NormalBorder()).BorderForeground(colorMuted).BorderBottom(true).Bold(true)
		ts.Selected = ts.Selected.Foreground(colorPrimary).Bold(false)
		t.SetStyles(ts)

		a.previewTbl = t
		if r.Total > 0 {
			a.previewInfo = fmt.Sprintf("~%s rows", formatCount(r.Total))
		}
		return a, nil

	case errMsg:
		a.statusErr = msg.err.Error()
		a.state = stateBrowse
		return a, nil

	case downloadDoneMsg:
		a.statusOK = fmt.Sprintf("Saved: %s", msg.path)
		a.state = stateDone
		return a, nil

	case DlCancelMsg:
		a.state = stateBrowse
		return a, nil

	case DlSubmitMsg:
		a.state = stateDownloading
		a.statusErr = ""
		a.statusOK = ""
		return a, tea.Batch(a.startDownload(msg), a.spinner.Tick)

	case tea.KeyMsg:
		if a.state == stateDownloadForm {
			var cmd tea.Cmd
			a.dlForm, cmd = a.dlForm.Update(msg)
			return a, cmd
		}

		switch msg.String() {
		case "q", "ctrl+c":
			return a, tea.Quit

		case "tab":
			a.focus = (a.focus + 1) % 3
			return a, nil

		case "shift+tab":
			a.focus = (a.focus + 2) % 3
			return a, nil

		case "enter":
			switch a.focus {
			case paneSchema:
				if sel := selectedItemTitle(a.schemaList); sel != "" {
					a.selectedSchema = sel
					a.selectedTable = ""
					a.focus = paneTable
					return a, a.loadTables(sel)
				}
			case paneTable:
				if sel := selectedItemTitle(a.tableList); sel != "" {
					a.selectedTable = sel
					a.focus = panePreview
					return a, a.loadPreview(a.selectedSchema, sel)
				}
			}

		case "d":
			if a.selectedSchema != "" && a.selectedTable != "" {
				a.dlForm = newDlForm(a.selectedSchema, a.selectedTable)
				a.state = stateDownloadForm
				return a, nil
			}

		case "esc":
			if a.state == stateDone {
				a.state = stateBrowse
				a.statusOK = ""
			}
			return a, nil
		}

		// Delegate keyboard events to the focused list.
		var cmd tea.Cmd
		switch a.focus {
		case paneSchema:
			a.schemaList, cmd = a.schemaList.Update(msg)
		case paneTable:
			a.tableList, cmd = a.tableList.Update(msg)
		case panePreview:
			a.previewTbl, cmd = a.previewTbl.Update(msg)
		}
		return a, cmd
	}

	// Forward spinner ticks when downloading.
	if a.state == stateDownloading {
		var cmd tea.Cmd
		a.spinner, cmd = a.spinner.Update(msg)
		return a, cmd
	}

	return a, nil
}

// View renders the full TUI.
func (a *App) View() string {
	if a.width == 0 {
		return "Loading…"
	}

	header := styleTitle.Render(" WRDS") + styleStatusBar.Render("  Wharton Research Data Services")
	footer := a.footerView()

	// Content area height.
	contentH := a.height - lipgloss.Height(header) - lipgloss.Height(footer) - 2

	schemaPanelW, tablePanelW, previewPanelW := a.panelWidths()

	schemaPanel := a.renderListPanel(a.schemaList, "Schemas", paneSchema, schemaPanelW, contentH)
	tablePanel := a.renderListPanel(a.tableList, fmt.Sprintf("Tables (%s)", a.selectedSchema), paneTable, tablePanelW, contentH)
	previewPanel := a.renderPreviewPanel(previewPanelW, contentH)

	body := lipgloss.JoinHorizontal(lipgloss.Top, schemaPanel, tablePanel, previewPanel)
	full := lipgloss.JoinVertical(lipgloss.Left, header, body, footer)

	if a.state == stateDownloadForm {
		overlay := a.dlForm.View(a.width)
		return overlayCenter(full, overlay, a.width, a.height)
	}
	if a.state == stateDownloading {
		msg := a.spinner.View() + "  Downloading…"
		return overlayCenter(full, stylePanelFocused.Padding(1, 3).Render(msg), a.width, a.height)
	}
	if a.state == stateDone {
		msg := styleSuccess.Render("✓ ") + a.statusOK + "\n\n" + styleStatusBar.Render("[esc] dismiss")
		return overlayCenter(full, stylePanelFocused.Padding(1, 3).Render(msg), a.width, a.height)
	}

	return full
}

func (a *App) footerView() string {
	keys := "[tab] switch pane  [enter] select  [d] download  [/] filter  [q] quit"
	status := ""
	if a.statusErr != "" {
		status = "  " + styleError.Render("Error: "+a.statusErr)
	}
	return styleStatusBar.Render(keys + status)
}

func (a *App) renderListPanel(l list.Model, title string, p pane, w, h int) string {
	l.SetSize(w-4, h-2)
	content := l.View()
	style := stylePanelBlurred
	if a.focus == p {
		style = stylePanelFocused
	}
	return style.Width(w - 2).Height(h).Render(content)
}

func (a *App) renderPreviewPanel(w, h int) string {
	var sb strings.Builder
	label := "Preview"
	if a.selectedSchema != "" && a.selectedTable != "" {
		label = fmt.Sprintf("Preview: %s.%s", a.selectedSchema, a.selectedTable)
	}
	sb.WriteString(stylePanelHeader.Render(label) + "\n")

	if len(a.previewTbl.Columns()) > 0 {
		a.previewTbl.SetHeight(h - 4)
		sb.WriteString(a.previewTbl.View())
		if a.previewInfo != "" {
			sb.WriteString("\n" + styleRowCount.Render(a.previewInfo))
		}
	} else if a.selectedTable != "" {
		sb.WriteString(styleStatusBar.Render("Loading…"))
	} else {
		sb.WriteString(styleStatusBar.Render("Select a table to preview rows"))
	}

	style := stylePanelBlurred
	if a.focus == panePreview {
		style = stylePanelFocused
	}
	return style.Width(w - 2).Height(h).Render(sb.String())
}

func (a *App) panelWidths() (int, int, int) {
	schema := 22
	table := 28
	preview := a.width - schema - table
	if preview < 30 {
		preview = 30
	}
	return schema, table, preview
}

func (a *App) previewHeight() int {
	return a.height - 4
}

func (a *App) resizePanels() {}

// -- helpers --

func selectedItemTitle(l list.Model) string {
	if sel := l.SelectedItem(); sel != nil {
		return sel.(item).title
	}
	return ""
}

func maxWidth(header string, rows [][]string, col, max int) int {
	w := len(header)
	for _, row := range rows {
		if col < len(row) && len(row[col]) > w {
			w = len(row[col])
		}
	}
	if w > max {
		return max
	}
	return w + 2
}

func formatCount(n int64) string {
	if n >= 1_000_000_000 {
		return fmt.Sprintf("%.1fB", float64(n)/1e9)
	}
	if n >= 1_000_000 {
		return fmt.Sprintf("%.1fM", float64(n)/1e6)
	}
	if n >= 1_000 {
		return fmt.Sprintf("%.1fK", float64(n)/1e3)
	}
	return fmt.Sprintf("%d", n)
}

// overlayCenter places overlay on top of base, centered.
func overlayCenter(base, overlay string, w, h int) string {
	_ = w
	_ = h
	// Simple approach: render overlay below header.
	lines := strings.Split(base, "\n")
	overlayLines := strings.Split(overlay, "\n")

	startRow := (len(lines) - len(overlayLines)) / 2
	if startRow < 0 {
		startRow = 0
	}

	for i, ol := range overlayLines {
		row := startRow + i
		if row < len(lines) {
			lineRunes := []rune(lines[row])
			olRunes := []rune(ol)
			startCol := (w - lipgloss.Width(ol)) / 2
			if startCol < 0 {
				startCol = 0
			}
			_ = lineRunes
			_ = olRunes
			_ = startCol
			lines[row] = ol
		}
	}
	return strings.Join(lines, "\n")
}
