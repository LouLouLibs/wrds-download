package tui

import (
	"os"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type loginField int

const (
	loginFieldSaved loginField = iota // "Login with saved credentials" button
	loginFieldUser
	loginFieldPassword
	loginFieldDatabase
	loginFieldSave
	loginFieldCount
)

const loginTextInputs = 3 // user, password, database

// LoginForm is the login dialog overlay shown when credentials are missing.
type LoginForm struct {
	inputs    [loginTextInputs]textinput.Model
	save      bool
	focused   loginField
	savedUser string // non-empty when saved credentials are available
	savedPw   string
	savedDB   string
}

// LoginSubmitMsg is sent when the user confirms the login form.
type LoginSubmitMsg struct {
	User     string
	Password string
	Database string
	Save     bool
}

// LoginCancelMsg is sent when the user cancels the login form.
type LoginCancelMsg struct{}

func newLoginForm() LoginForm {
	f := LoginForm{}

	f.inputs[loginFieldUser-1] = textinput.New()
	f.inputs[loginFieldUser-1].Placeholder = "WRDS username"
	f.inputs[loginFieldUser-1].CharLimit = 128

	f.inputs[loginFieldPassword-1] = textinput.New()
	f.inputs[loginFieldPassword-1].Placeholder = "WRDS password"
	f.inputs[loginFieldPassword-1].CharLimit = 128
	f.inputs[loginFieldPassword-1].EchoMode = textinput.EchoPassword
	f.inputs[loginFieldPassword-1].EchoCharacter = '*'

	f.inputs[loginFieldDatabase-1] = textinput.New()
	f.inputs[loginFieldDatabase-1].Placeholder = "wrds"
	f.inputs[loginFieldDatabase-1].CharLimit = 128
	f.inputs[loginFieldDatabase-1].SetValue("wrds")

	// Check for saved credentials in env (set by config.ApplyCredentials).
	f.savedUser = os.Getenv("PGUSER")
	f.savedPw = os.Getenv("PGPASSWORD")
	f.savedDB = os.Getenv("PGDATABASE")
	if f.savedDB == "" {
		f.savedDB = "wrds"
	}

	f.save = true

	if f.hasSaved() {
		f.focused = loginFieldSaved
	} else {
		f.focused = loginFieldUser
		f.inputs[loginFieldUser-1].Focus()
	}
	return f
}

func (f *LoginForm) hasSaved() bool {
	return f.savedUser != "" && f.savedPw != ""
}

// inputIndex maps a loginField to the inputs array index.
// Returns -1 for non-input fields (saved, save).
func inputIndex(field loginField) int {
	switch field {
	case loginFieldUser, loginFieldPassword, loginFieldDatabase:
		return int(field) - 1
	}
	return -1
}

func (f *LoginForm) blurCurrent() {
	if idx := inputIndex(f.focused); idx >= 0 {
		f.inputs[idx].Blur()
	}
}

func (f *LoginForm) focusCurrent() tea.Cmd {
	if idx := inputIndex(f.focused); idx >= 0 {
		f.inputs[idx].Focus()
		return textinput.Blink
	}
	return nil
}

func (f *LoginForm) advance(delta int) tea.Cmd {
	f.blurCurrent()
	start := loginFieldSaved
	if !f.hasSaved() {
		start = loginFieldUser
	}
	count := int(loginFieldCount) - int(start)
	pos := (int(f.focused) - int(start) + delta%count + count) % count
	f.focused = loginField(pos + int(start))
	return f.focusCurrent()
}

func (f LoginForm) submit() tea.Cmd {
	user := strings.TrimSpace(f.inputs[loginFieldUser-1].Value())
	pw := f.inputs[loginFieldPassword-1].Value()
	if user == "" || pw == "" {
		return nil
	}
	database := strings.TrimSpace(f.inputs[loginFieldDatabase-1].Value())
	if database == "" {
		database = "wrds"
	}
	return func() tea.Msg {
		return LoginSubmitMsg{User: user, Password: pw, Database: database, Save: f.save}
	}
}

func (f LoginForm) Update(msg tea.Msg) (LoginForm, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "esc":
			return f, func() tea.Msg { return LoginCancelMsg{} }

		case "enter":
			if f.focused == loginFieldSaved {
				// Connect using saved credentials.
				return f, func() tea.Msg {
					return LoginSubmitMsg{
						User:     f.savedUser,
						Password: f.savedPw,
						Database: f.savedDB,
						Save:     false,
					}
				}
			}
			if f.focused == loginFieldSave {
				return f, f.submit()
			}
			// Advance to next field.
			cmd := f.advance(1)
			return f, cmd

		case "tab", "down":
			cmd := f.advance(1)
			return f, cmd

		case "shift+tab", "up":
			cmd := f.advance(-1)
			return f, cmd

		case " ":
			if f.focused == loginFieldSave {
				f.save = !f.save
				return f, nil
			}
		}
	}

	// Forward to focused text input.
	if idx := inputIndex(f.focused); idx >= 0 {
		var cmd tea.Cmd
		f.inputs[idx], cmd = f.inputs[idx].Update(msg)
		return f, cmd
	}
	return f, nil
}

func (f LoginForm) View(width int, errMsg string) string {
	var sb strings.Builder

	title := stylePanelHeader.Render("WRDS Login")
	sb.WriteString(title + "\n\n")

	// "Login with saved credentials" button.
	if f.hasSaved() {
		btnLabel := "Login as " + f.savedUser
		btnStyle := lipgloss.NewStyle().Foreground(colorMuted)
		if f.focused == loginFieldSaved {
			btnStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#FFFFFF")).
				Background(colorFocus).
				Padding(0, 1)
			btnLabel += "  [enter]"
		}
		sb.WriteString(btnStyle.Render(btnLabel) + "\n\n")
		sb.WriteString(lipgloss.NewStyle().Foreground(colorMuted).Render("── or enter credentials manually ──") + "\n\n")
	}

	labels := []string{"Username", "Password", "Database"}
	fields := []loginField{loginFieldUser, loginFieldPassword, loginFieldDatabase}
	for i, label := range labels {
		style := lipgloss.NewStyle().Foreground(colorMuted)
		if fields[i] == f.focused {
			style = lipgloss.NewStyle().Foreground(colorFocus)
		}
		sb.WriteString(style.Render(label+"  ") + "\n")
		sb.WriteString(f.inputs[i].View() + "\n\n")
	}

	// Save toggle.
	check := "[ ]"
	if f.save {
		check = "[x]"
	}
	saveStyle := lipgloss.NewStyle().Foreground(colorMuted)
	if f.focused == loginFieldSave {
		saveStyle = lipgloss.NewStyle().Foreground(colorFocus)
	}
	sb.WriteString(saveStyle.Render(check+" Save to ~/.config/wrds-dl/credentials") + "\n\n")

	if errMsg != "" {
		maxLen := 52
		if len(errMsg) > maxLen {
			errMsg = errMsg[:maxLen-1] + "…"
		}
		sb.WriteString(styleError.Render("Error: "+errMsg) + "\n\n")
	}

	hint := styleStatusBar.Render("[tab] next field   [enter] submit   [esc] quit")
	sb.WriteString(hint)

	content := sb.String()
	boxWidth := 60
	if boxWidth > width-4 {
		boxWidth = width - 4
	}
	if boxWidth < 40 {
		boxWidth = 40
	}

	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(colorFocus).
		Padding(1, 2).
		Width(boxWidth).
		Render(content)

	return lipgloss.Place(width, 24, lipgloss.Center, lipgloss.Center, box)
}
