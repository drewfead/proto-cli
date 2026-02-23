package bubbles

import (
	"strings"

	"github.com/charmbracelet/bubbles/textarea"
	tea "github.com/charmbracelet/bubbletea"
)

// ─── Vi-modal input ───────────────────────────────────────────────────────────

type vimMode int

const (
	vimModeCommand vimMode = iota
	vimModeInsert
)

// vimInput is a vi-modal textarea with live validation.
//
// Key bindings (insert mode):
//   - Esc    return to command mode
//   - Tab    insert a literal tab character
//   - Enter  insert a newline
//
// Key bindings (command mode):
//   - i      enter insert mode
//   - r      reset content to the default value
//   - ↑↓/Tab navigate to adjacent form fields
//   - Enter  submit the form
type vimInput struct {
	area         textarea.Model
	styles       Styles
	mode         vimMode
	defaultValue string
	validate     func(string) error
	validErr     string
	validLabel   string // shown in "✓ label" indicator when validation passes
}

// NewVimInput returns a vi-modal textarea FormControl.
// The editor is 10 lines tall and 60 characters wide by default.
// defaultValue pre-populates the editor; pass "" for an empty editor.
// validate is called on every keystroke in insert mode; pass nil to skip validation.
// validLabel is the text shown in the "✓ label" indicator when valid (e.g. "valid JSON").
func NewVimInput(defaultValue string, styles Styles, validate func(string) error, validLabel string) FormControl {
	ta := textarea.New()
	ta.Placeholder = "..."
	ta.SetWidth(60)
	ta.SetHeight(10)
	ta.CharLimit = 0
	if defaultValue != "" {
		ta.SetValue(defaultValue)
	}
	c := &vimInput{area: ta, styles: styles, defaultValue: defaultValue, validate: validate, validLabel: validLabel}
	c.runValidate()
	return c
}

// IsMultiline delegates Enter to the control only while in insert mode,
// so Enter inserts newlines instead of submitting the form.
func (c *vimInput) IsMultiline() bool {
	return c.mode == vimModeInsert
}

// HelpText returns context-sensitive help for the current vi mode.
func (c *vimInput) HelpText() string {
	if c.mode == vimModeInsert {
		return "Esc: command mode"
	}
	return FormHelpText(
		KeyBind{"↑↓/Tab", "navigate"},
		KeyBind{"Enter", "submit"},
		KeyBind{"i", "insert"},
		KeyBind{"r", "reset"},
	)
}

// CapturesKey claims Esc, cursor-movement, and Tab in insert mode so they
// reach this control rather than triggering form-level navigation.
func (c *vimInput) CapturesKey(key string) bool {
	if c.mode == vimModeInsert {
		return key == "esc" || key == "up" || key == "down" || key == "tab"
	}
	return false
}

// Focus moves keyboard focus to this control but stays in command mode.
// The user must press 'i' to enter insert mode and begin editing.
func (c *vimInput) Focus() tea.Cmd { return nil }

// Blur returns to command mode and blurs the underlying textarea.
func (c *vimInput) Blur() {
	c.mode = vimModeCommand
	c.area.Blur()
}

func (c *vimInput) Value() string { return c.area.Value() }

func (c *vimInput) View() string {
	var sb strings.Builder
	sb.WriteString(c.area.View())
	sb.WriteString("\n")

	// Mode indicator
	if c.mode == vimModeInsert {
		sb.WriteString(c.styles.Help.Render("-- INSERT --"))
	} else {
		sb.WriteString(c.styles.Help.Render("-- COMMAND --"))
	}

	// Validation indicator (only shown when validate is configured and value is non-empty)
	if c.validate != nil {
		if raw := strings.TrimSpace(c.area.Value()); raw != "" {
			sb.WriteString("  ")
			if c.validErr != "" {
				sb.WriteString(c.styles.JSONInvalid.Render("✗  " + c.validErr))
			} else {
				sb.WriteString(c.styles.JSONValid.Render("✓  " + c.validLabel))
			}
		}
	}

	return sb.String()
}

func (c *vimInput) Update(msg tea.Msg) tea.Cmd {
	key, ok := msg.(tea.KeyMsg)
	if !ok {
		if c.mode == vimModeInsert {
			var cmd tea.Cmd
			c.area, cmd = c.area.Update(msg)
			return cmd
		}
		return nil
	}

	switch c.mode {
	case vimModeInsert:
		switch key.String() {
		case "esc":
			c.mode = vimModeCommand
			c.area.Blur()
			return nil
		case "tab":
			// Send a literal tab rune; textarea's native tab handling does not
			// insert a tab character, so we convert the key message manually.
			var cmd tea.Cmd
			c.area, cmd = c.area.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'\t'}})
			c.runValidate()
			return cmd
		default:
			var cmd tea.Cmd
			c.area, cmd = c.area.Update(msg)
			c.runValidate()
			return cmd
		}

	case vimModeCommand:
		switch key.String() {
		case "i":
			c.mode = vimModeInsert
			return c.area.Focus()
		case "r":
			c.area.SetValue(c.defaultValue)
			c.runValidate()
		}
	}
	return nil
}

func (c *vimInput) runValidate() {
	if c.validate == nil {
		c.validErr = ""
		return
	}
	raw := strings.TrimSpace(c.area.Value())
	if raw == "" {
		c.validErr = ""
		return
	}
	if err := c.validate(raw); err != nil {
		c.validErr = err.Error()
	} else {
		c.validErr = ""
	}
}

