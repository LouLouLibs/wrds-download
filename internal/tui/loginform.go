package tui

import (
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type loginField int

const (
	loginFieldUser loginField = iota
	loginFieldPassword
	loginFieldDatabase
	loginFieldSave
	loginFieldCount
)

const loginTextInputs = 3 // number of text input fields (before the save toggle)

// LoginForm is the login dialog overlay shown when credentials are missing.
type LoginForm struct {
	inputs  [loginTextInputs]textinput.Model
	save    bool
	focused loginField
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

	f.inputs[loginFieldUser] = textinput.New()
	f.inputs[loginFieldUser].Placeholder = "WRDS username"
	f.inputs[loginFieldUser].CharLimit = 128

	f.inputs[loginFieldPassword] = textinput.New()
	f.inputs[loginFieldPassword].Placeholder = "WRDS password"
	f.inputs[loginFieldPassword].CharLimit = 128
	f.inputs[loginFieldPassword].EchoMode = textinput.EchoPassword
	f.inputs[loginFieldPassword].EchoCharacter = '*'

	f.inputs[loginFieldDatabase] = textinput.New()
	f.inputs[loginFieldDatabase].Placeholder = "wrds"
	f.inputs[loginFieldDatabase].CharLimit = 128
	f.inputs[loginFieldDatabase].SetValue("wrds")

	f.save = true
	f.inputs[loginFieldUser].Focus()
	return f
}

func (f LoginForm) Update(msg tea.Msg) (LoginForm, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "esc":
			return f, func() tea.Msg { return LoginCancelMsg{} }

		case "enter":
			if f.focused == loginFieldSave {
				// Submit
				user := strings.TrimSpace(f.inputs[loginFieldUser].Value())
				pw := f.inputs[loginFieldPassword].Value()
				if user == "" || pw == "" {
					return f, nil
				}
				database := strings.TrimSpace(f.inputs[loginFieldDatabase].Value())
				if database == "" {
					database = "wrds"
				}
				return f, func() tea.Msg {
					return LoginSubmitMsg{User: user, Password: pw, Database: database, Save: f.save}
				}
			}
			// Advance to next field
			if int(f.focused) < loginTextInputs {
				f.inputs[f.focused].Blur()
			}
			f.focused++
			if int(f.focused) < loginTextInputs {
				f.inputs[f.focused].Focus()
				return f, textinput.Blink
			}
			return f, nil

		case "tab", "down":
			if int(f.focused) < loginTextInputs {
				f.inputs[f.focused].Blur()
			}
			f.focused = loginField((int(f.focused) + 1) % int(loginFieldCount))
			if int(f.focused) < loginTextInputs {
				f.inputs[f.focused].Focus()
				return f, textinput.Blink
			}
			return f, nil

		case "shift+tab", "up":
			if int(f.focused) < loginTextInputs {
				f.inputs[f.focused].Blur()
			}
			f.focused = loginField((int(f.focused) + int(loginFieldCount) - 1) % int(loginFieldCount))
			if int(f.focused) < loginTextInputs {
				f.inputs[f.focused].Focus()
				return f, textinput.Blink
			}
			return f, nil

		case " ":
			if f.focused == loginFieldSave {
				f.save = !f.save
				return f, nil
			}
		}
	}

	// Forward to focused text input
	if int(f.focused) < loginTextInputs {
		var cmd tea.Cmd
		f.inputs[f.focused], cmd = f.inputs[f.focused].Update(msg)
		return f, cmd
	}
	return f, nil
}

func (f LoginForm) View(width int, errMsg string) string {
	var sb strings.Builder

	title := stylePanelHeader.Render("WRDS Login")
	sb.WriteString(title + "\n\n")

	labels := []string{"Username", "Password", "Database"}
	for i, label := range labels {
		style := lipgloss.NewStyle().Foreground(colorMuted)
		if loginField(i) == f.focused {
			style = lipgloss.NewStyle().Foreground(colorFocus)
		}
		sb.WriteString(style.Render(label+"  ") + "\n")
		sb.WriteString(f.inputs[i].View() + "\n\n")
	}

	// Save toggle
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
