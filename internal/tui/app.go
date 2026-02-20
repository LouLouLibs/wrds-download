package tui

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/eloualiche/wrds-download/internal/config"
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
	stateLogin appState = iota
	stateBrowse
	stateDatabaseSelect
	stateDownloadForm
	stateDownloading
	stateDone
)

// -- Tea messages --

type schemasLoadedMsg struct{ schemas []db.Schema }
type tablesLoadedMsg struct{ tables []db.Table }
type metaLoadedMsg struct{ meta *db.TableMeta }
type errMsg struct{ err error }
type downloadDoneMsg struct{ path string }
type tickMsg time.Time
type loginSuccessMsg struct{ client *db.Client }
type loginFailMsg struct{ err error }
type databasesLoadedMsg struct{ databases []string }
type databaseSwitchedMsg struct{ client *db.Client }
type databaseSwitchFailMsg struct{ err error }

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

	schemaList       list.Model
	tableList        list.Model
	previewMeta      *db.TableMeta
	previewScroll    int
	previewFilter    textinput.Model
	previewFiltering bool

	loginForm LoginForm
	loginErr  string
	dlForm    DlForm
	dbList    list.Model
	spinner   spinner.Model
	statusOK  string
	statusErr string

	currentDatabase string
	selectedSchema  string
	selectedTable   string
}

func newPreviewFilter() textinput.Model {
	pf := textinput.New()
	pf.Prompt = "/ "
	pf.Placeholder = "filter columns…"
	pf.CharLimit = 64
	return pf
}

// NewApp constructs the root model.
func NewApp(client *db.Client) *App {
	del := list.NewDefaultDelegate()
	del.ShowDescription = false
	del.SetSpacing(0)

	schemaList := list.New(nil, del, 0, 0)
	schemaList.Title = "Schemas"
	schemaList.SetShowStatusBar(false)
	schemaList.SetFilteringEnabled(true)
	schemaList.DisableQuitKeybindings()
	schemaList.Styles.TitleBar = schemaList.Styles.TitleBar.Padding(0, 0, 0, 2)

	tableList := list.New(nil, del, 0, 0)
	tableList.Title = "Tables"
	tableList.SetShowStatusBar(false)
	tableList.SetFilteringEnabled(true)
	tableList.DisableQuitKeybindings()
	tableList.Styles.TitleBar = tableList.Styles.TitleBar.Padding(0, 0, 0, 2)

	dbList := list.New(nil, del, 0, 0)
	dbList.Title = "Databases"
	dbList.SetShowStatusBar(false)
	dbList.SetFilteringEnabled(true)
	dbList.DisableQuitKeybindings()
	dbList.Styles.TitleBar = dbList.Styles.TitleBar.Padding(0, 0, 0, 2)

	sp := spinner.New()
	sp.Spinner = spinner.Dot

	return &App{
		client:          client,
		currentDatabase: os.Getenv("PGDATABASE"),
		schemaList:      schemaList,
		tableList:       tableList,
		dbList:          dbList,
		spinner:         sp,
		previewFilter:   newPreviewFilter(),
		focus:           paneSchema,
		state:           stateBrowse,
	}
}

// NewAppNoClient creates an App in login state (no DB connection yet).
func NewAppNoClient() *App {
	del := list.NewDefaultDelegate()
	del.ShowDescription = false
	del.SetSpacing(0)

	schemaList := list.New(nil, del, 0, 0)
	schemaList.Title = "Schemas"
	schemaList.SetShowStatusBar(false)
	schemaList.SetFilteringEnabled(true)
	schemaList.DisableQuitKeybindings()
	schemaList.Styles.TitleBar = schemaList.Styles.TitleBar.Padding(0, 0, 0, 2)

	tableList := list.New(nil, del, 0, 0)
	tableList.Title = "Tables"
	tableList.SetShowStatusBar(false)
	tableList.SetFilteringEnabled(true)
	tableList.DisableQuitKeybindings()
	tableList.Styles.TitleBar = tableList.Styles.TitleBar.Padding(0, 0, 0, 2)

	dbList := list.New(nil, del, 0, 0)
	dbList.Title = "Databases"
	dbList.SetShowStatusBar(false)
	dbList.SetFilteringEnabled(true)
	dbList.DisableQuitKeybindings()
	dbList.Styles.TitleBar = dbList.Styles.TitleBar.Padding(0, 0, 0, 2)

	sp := spinner.New()
	sp.Spinner = spinner.Dot

	return &App{
		schemaList:    schemaList,
		tableList:     tableList,
		dbList:        dbList,
		spinner:       sp,
		previewFilter: newPreviewFilter(),
		focus:         paneSchema,
		state:         stateLogin,
		loginForm:     newLoginForm(),
	}
}

// Init loads schemas on startup, or starts login form blink if in login state.
func (a *App) Init() tea.Cmd {
	if a.state == stateLogin {
		return textinput.Blink
	}
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

func (a *App) loadMeta(schema, tbl string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		meta, err := a.client.TableMeta(ctx, schema, tbl)
		if err != nil {
			return errMsg{err}
		}
		return metaLoadedMsg{meta}
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

func (a *App) attemptLogin(msg LoginSubmitMsg) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		client, err := db.NewWithCredentials(ctx, msg.User, msg.Password, msg.Database)
		if err != nil {
			return loginFailMsg{err}
		}
		if msg.Save {
			_ = config.SaveCredentials(msg.User, msg.Password, msg.Database)
		}
		return loginSuccessMsg{client}
	}
}

func (a *App) loadDatabases() tea.Cmd {
	return func() tea.Msg {
		dbs, err := a.client.Databases(context.Background())
		if err != nil {
			return errMsg{err}
		}
		return databasesLoadedMsg{dbs}
	}
}

func (a *App) switchDatabase(name string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		user := os.Getenv("PGUSER")
		password := os.Getenv("PGPASSWORD")
		client, err := db.NewWithCredentials(ctx, user, password, name)
		if err != nil {
			return databaseSwitchFailMsg{err}
		}
		return databaseSwitchedMsg{client}
	}
}

// friendlyError extracts a short, readable message from verbose pgx errors.
func friendlyError(err error) string {
	s := err.Error()
	// pgx errors look like: "ping: failed to connect to `host=... user=...`: <reason>"
	// Extract just the reason after the last colon-space following the backtick-quoted section.
	if idx := strings.LastIndex(s, "`: "); idx != -1 {
		return s[idx+3:]
	}
	// Fall back to stripping common prefixes.
	for _, prefix := range []string{"ping: ", "pgxpool.New: "} {
		s = strings.TrimPrefix(s, prefix)
	}
	return s
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
		a.previewMeta = nil
		a.previewScroll = 0
		a.previewFilter.SetValue("")
		a.previewFiltering = false
		return a, nil

	case metaLoadedMsg:
		a.previewMeta = msg.meta
		a.previewScroll = 0
		a.previewFilter.SetValue("")
		a.previewFiltering = false
		return a, nil

	case LoginSubmitMsg:
		a.loginErr = ""
		return a, a.attemptLogin(msg)

	case LoginCancelMsg:
		return a, tea.Quit

	case loginSuccessMsg:
		a.client = msg.client
		a.currentDatabase = os.Getenv("PGDATABASE")
		a.state = stateBrowse
		return a, tea.Batch(a.loadSchemas(), a.spinner.Tick)

	case loginFailMsg:
		a.loginErr = friendlyError(msg.err)
		a.state = stateLogin
		return a, nil

	case databasesLoadedMsg:
		items := make([]list.Item, len(msg.databases))
		for i, d := range msg.databases {
			items[i] = item{d}
		}
		a.dbList.SetItems(items)
		a.state = stateDatabaseSelect
		return a, nil

	case databaseSwitchedMsg:
		a.client.Close()
		a.client = msg.client
		a.currentDatabase = os.Getenv("PGDATABASE")
		a.selectedSchema = ""
		a.selectedTable = ""
		a.previewMeta = nil
		a.previewScroll = 0
		a.previewFilter.SetValue("")
		a.tableList.SetItems(nil)
		a.state = stateBrowse
		return a, a.loadSchemas()

	case databaseSwitchFailMsg:
		a.statusErr = friendlyError(msg.err)
		a.state = stateBrowse
		return a, nil

	case errMsg:
		a.statusErr = friendlyError(msg.err)
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

	case list.FilterMatchesMsg:
		// Route async filter results back to the list that initiated filtering.
		var cmd tea.Cmd
		switch {
		case a.schemaList.FilterState() == list.Filtering:
			a.schemaList, cmd = a.schemaList.Update(msg)
		case a.tableList.FilterState() == list.Filtering:
			a.tableList, cmd = a.tableList.Update(msg)
		case a.dbList.FilterState() == list.Filtering:
			a.dbList, cmd = a.dbList.Update(msg)
		}
		return a, cmd

	case tea.KeyMsg:
		if a.state == stateLogin {
			var cmd tea.Cmd
			a.loginForm, cmd = a.loginForm.Update(msg)
			return a, cmd
		}
		if a.state == stateDownloadForm {
			var cmd tea.Cmd
			a.dlForm, cmd = a.dlForm.Update(msg)
			return a, cmd
		}
		if a.state == stateDatabaseSelect {
			if a.dbList.FilterState() == list.Filtering {
				var cmd tea.Cmd
				a.dbList, cmd = a.dbList.Update(msg)
				return a, cmd
			}
			switch msg.String() {
			case "esc":
				a.state = stateBrowse
				return a, nil
			case "enter":
				if sel := selectedItemTitle(a.dbList); sel != "" {
					a.state = stateDownloading
					return a, tea.Batch(a.switchDatabase(sel), a.spinner.Tick)
				}
			}
			var cmd tea.Cmd
			a.dbList, cmd = a.dbList.Update(msg)
			return a, cmd
		}

		// Preview column filter: intercept all keys when active.
		if a.focus == panePreview && a.previewFiltering {
			switch msg.String() {
			case "esc":
				a.previewFiltering = false
				a.previewFilter.SetValue("")
				a.previewFilter.Blur()
				return a, nil
			case "enter":
				a.previewFiltering = false
				a.previewFilter.Blur()
				return a, nil
			}
			var cmd tea.Cmd
			a.previewFilter, cmd = a.previewFilter.Update(msg)
			a.previewScroll = 0
			return a, cmd
		}

		switch msg.String() {
		case "q", "ctrl+c":
			if a.focusedListFiltering() {
				break // let list handle it
			}
			return a, tea.Quit

		case "tab":
			if a.focusedListFiltering() {
				break
			}
			a.statusErr = ""
			a.focus = (a.focus + 1) % 3
			return a, nil

		case "shift+tab":
			if a.focusedListFiltering() {
				break
			}
			a.statusErr = ""
			a.focus = (a.focus + 2) % 3
			return a, nil

		case "right", "l":
			if a.focusedListFiltering() {
				break
			}
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
					a.previewMeta = nil
					a.previewScroll = 0
					a.previewFilter.SetValue("")
					a.focus = panePreview
					return a, a.loadMeta(a.selectedSchema, sel)
				}
			}
			return a, nil

		case "left", "h":
			if a.focusedListFiltering() {
				break
			}
			if a.focus > paneSchema {
				a.focus--
			}
			return a, nil

		case "d":
			if a.focusedListFiltering() {
				break
			}
			if a.selectedSchema != "" && a.selectedTable != "" {
				a.dlForm = newDlForm(a.selectedSchema, a.selectedTable)
				a.state = stateDownloadForm
				return a, nil
			}

		case "b":
			if a.focusedListFiltering() {
				break
			}
			return a, a.loadDatabases()

		case "esc":
			if a.focusedListFiltering() {
				break // let list cancel filter
			}
			if a.state == stateDone {
				a.state = stateBrowse
				a.statusOK = ""
			}
			return a, nil
		}

		// All other keys (including enter, /, letters) go to the focused list/pane.
		var cmd tea.Cmd
		switch a.focus {
		case paneSchema:
			a.schemaList, cmd = a.schemaList.Update(msg)
		case paneTable:
			a.tableList, cmd = a.tableList.Update(msg)
		case panePreview:
			switch msg.String() {
			case "/":
				a.previewFiltering = true
				a.previewFilter.Focus()
				cmd = textinput.Blink
			case "j", "down":
				cols := a.filteredColumns()
				if a.previewScroll < len(cols)-1 {
					a.previewScroll++
				}
			case "k", "up":
				if a.previewScroll > 0 {
					a.previewScroll--
				}
			}
		}
		return a, cmd
	}

	// Forward cursor blink messages to the active text input.
	if a.previewFiltering {
		var cmd tea.Cmd
		a.previewFilter, cmd = a.previewFilter.Update(msg)
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

	if a.state == stateLogin {
		return a.loginView()
	}

	dbLabel := ""
	if a.currentDatabase != "" {
		dbLabel = "  db:" + a.currentDatabase
	}
	header := styleTitle.Render(" WRDS") + styleStatusBar.Render("  Wharton Research Data Services"+dbLabel)
	footer := a.footerView()

	// Content area height.
	contentH := a.height - lipgloss.Height(header) - lipgloss.Height(footer) - 2

	schemaPanelW, tablePanelW, previewPanelW := a.panelWidths()

	schemaPanel := a.renderListPanel(a.schemaList, "Schemas", paneSchema, schemaPanelW, contentH, 1)
	tablePanel := a.renderListPanel(a.tableList, fmt.Sprintf("Tables (%s)", a.selectedSchema), paneTable, tablePanelW, contentH, 1)
	previewPanel := a.renderPreviewPanel(previewPanelW, contentH)

	body := lipgloss.JoinHorizontal(lipgloss.Top, schemaPanel, tablePanel, previewPanel)
	full := lipgloss.JoinVertical(lipgloss.Left, header, body, footer)

	if a.state == stateDatabaseSelect {
		a.dbList.SetSize(40, a.height/2)
		content := a.dbList.View()
		hint := styleStatusBar.Render("[enter] switch   [esc] cancel   [/] filter")
		box := lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(colorFocus).
			Padding(1, 2).
			Render(content + "\n" + hint)
		return overlayCenter(full, box, a.width, a.height)
	}
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

func (a *App) loginView() string {
	var sb strings.Builder
	sb.WriteString(styleTitle.Render(" WRDS") + styleStatusBar.Render("  Wharton Research Data Services") + "\n\n")
	sb.WriteString(a.loginForm.View(a.width, a.loginErr))
	return lipgloss.Place(a.width, a.height, lipgloss.Center, lipgloss.Center, sb.String())
}

func (a *App) footerView() string {
	keys := "[tab] pane  [→/l] select  [←/h] back  [d] download  [b] databases  [/] filter  [q] quit"
	footer := styleStatusBar.Render(keys)
	if a.statusErr != "" {
		errText := a.statusErr
		maxLen := a.width - 12
		if maxLen > 0 && len(errText) > maxLen {
			errText = errText[:maxLen-1] + "…"
		}
		errBar := lipgloss.NewStyle().
			Foreground(lipgloss.Color("#FFFFFF")).
			Background(colorError).
			Width(a.width).
			Padding(0, 1).
			Render("Error: " + errText)
		footer = errBar + "\n" + footer
	}
	return footer
}

func (a *App) renderListPanel(l list.Model, title string, p pane, w, h, mr int) string {
	l.SetSize(w-2, h-2)
	content := l.View()
	style := stylePanelBlurred
	if a.focus == p {
		style = stylePanelFocused
	}
	return style.Width(w - 2).Height(h).MarginRight(mr).Render(content)
}

func (a *App) renderPreviewPanel(w, h int) string {
	var sb strings.Builder
	label := "Preview"
	if a.selectedSchema != "" && a.selectedTable != "" {
		label = fmt.Sprintf("Preview: %s.%s", a.selectedSchema, a.selectedTable)
	}
	sb.WriteString(stylePanelHeader.Render(label) + "\n")

	contentW := w - 4 // panel border + internal padding

	if a.previewMeta != nil {
		meta := a.previewMeta

		// Stats line: "~245.3M rows · 1.2 GB"
		var stats []string
		if meta.RowCount > 0 {
			stats = append(stats, "~"+formatCount(meta.RowCount)+" rows")
		}
		if meta.Size != "" {
			stats = append(stats, meta.Size)
		}
		if len(stats) > 0 {
			sb.WriteString(styleRowCount.Render(strings.Join(stats, " · ")) + "\n")
		}
		if meta.Comment != "" {
			sb.WriteString(styleStatusBar.Render(meta.Comment) + "\n")
		}

		// Filter bar
		if a.previewFiltering {
			sb.WriteString(a.previewFilter.View() + "\n")
		} else if a.previewFilter.Value() != "" {
			sb.WriteString(styleStatusBar.Render("/ "+a.previewFilter.Value()) + "\n")
		}

		cols := a.filteredColumns()

		if len(cols) > 0 {
			// Calculate column widths from data.
			nameW, typeW := len("Column"), len("Type")
			for _, c := range cols {
				if len(c.Name) > nameW {
					nameW = len(c.Name)
				}
				if len(c.DataType) > typeW {
					typeW = len(c.DataType)
				}
			}
			if nameW > 22 {
				nameW = 22
			}
			if typeW > 20 {
				typeW = 20
			}
			descW := contentW - nameW - typeW - 4 // 2-char gaps
			if descW < 8 {
				descW = 8
			}

			// Column header
			hdr := fmt.Sprintf("%-*s  %-*s  %-*s", nameW, "Column", typeW, "Type", descW, "Description")
			sb.WriteString(styleCellHeader.Render(truncStr(hdr, contentW)) + "\n")
			sb.WriteString(lipgloss.NewStyle().Foreground(colorMuted).Render(strings.Repeat("─", contentW)) + "\n")

			// How many rows fit?
			usedLines := lipgloss.Height(sb.String())
			footerLines := 1
			availRows := h - usedLines - footerLines - 2
			if availRows < 1 {
				availRows = 1
			}

			start := a.previewScroll
			end := start + availRows
			if end > len(cols) {
				end = len(cols)
			}

			for i := start; i < end; i++ {
				c := cols[i]
				line := fmt.Sprintf("%-*s  %-*s  %s",
					nameW, truncStr(c.Name, nameW),
					typeW, truncStr(c.DataType, typeW),
					truncStr(c.Description, descW))
				style := styleCellNormal
				if i%2 == 0 {
					style = style.Foreground(lipgloss.Color("#D1D5DB"))
				}
				sb.WriteString(style.Render(line) + "\n")
			}
		}

		// Column count footer
		total := len(meta.Columns)
		shown := len(cols)
		countStr := fmt.Sprintf("%d columns", total)
		if shown < total {
			countStr = fmt.Sprintf("%d/%d columns", shown, total)
		}
		sb.WriteString(styleRowCount.Render(countStr))

	} else if a.selectedTable != "" {
		sb.WriteString(styleStatusBar.Render("Loading…"))
	} else {
		sb.WriteString(styleStatusBar.Render("Select a table to preview"))
	}

	style := stylePanelBlurred
	if a.focus == panePreview {
		style = stylePanelFocused
	}
	return style.Width(w - 2).Height(h).Render(sb.String())
}

func (a *App) panelWidths() (int, int, int) {
	schema := 24
	tbl := 30
	margins := 2 // MarginRight(1) on schema + table panels
	preview := a.width - schema - tbl - margins
	if preview < 30 {
		preview = 30
	}
	return schema, tbl, preview
}

func (a *App) resizePanels() {}

// focusedListFiltering returns true if the currently focused list is in filter mode.
func (a *App) focusedListFiltering() bool {
	switch a.focus {
	case paneSchema:
		return a.schemaList.FilterState() == list.Filtering
	case paneTable:
		return a.tableList.FilterState() == list.Filtering
	}
	return false
}

// filteredColumns returns the columns matching the current filter text.
func (a *App) filteredColumns() []db.ColumnMeta {
	if a.previewMeta == nil {
		return nil
	}
	filter := strings.ToLower(a.previewFilter.Value())
	if filter == "" {
		return a.previewMeta.Columns
	}
	var out []db.ColumnMeta
	for _, col := range a.previewMeta.Columns {
		if strings.Contains(strings.ToLower(col.Name), filter) ||
			strings.Contains(strings.ToLower(col.Description), filter) {
			out = append(out, col)
		}
	}
	return out
}

// Err returns the last error message (login or status), if any.
func (a *App) Err() string {
	if a.loginErr != "" {
		return a.loginErr
	}
	return a.statusErr
}

// -- helpers --

func selectedItemTitle(l list.Model) string {
	if sel := l.SelectedItem(); sel != nil {
		return sel.(item).title
	}
	return ""
}

func truncStr(s string, max int) string {
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	if max <= 1 {
		return "…"
	}
	return string(r[:max-1]) + "…"
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
