package bubbles

import (
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	protocli "github.com/drewfead/proto-cli"
	"google.golang.org/protobuf/proto"
)

// FormControl is the interface for TUI form input widgets.
// Implementations are responsible for rendering, focus management, and
// producing a string value consumed by the field's Setter or Appender.
//
// For complex message fields that need to apply values directly to the proto
// message (bypassing the string-based Setter), also implement FieldApplier.
type FormControl interface {
	View() string
	Focus() tea.Cmd
	Blur()
	Value() string
	Update(msg tea.Msg) tea.Cmd
}

// FieldApplier is an optional interface for FormControls that directly apply
// their value to a proto message. Useful for custom controls handling complex
// message types where encoding to a string is not practical.
// If a control implements FieldApplier, submitForm calls Apply instead of Setter.
type FieldApplier interface {
	Apply(msg proto.Message) error
}

// KeyCapturer is an optional interface for FormControls that need to intercept
// keys the form would otherwise consume for field navigation (up/down/tab).
// When the focused control's CapturesKey returns true the key is delegated
// directly to the control rather than triggering field navigation.
type KeyCapturer interface {
	CapturesKey(key string) bool
}

// MultilineInput is an optional interface for FormControls that accept
// multi-line text (e.g. a JSON editor). When the focused control implements
// this interface and IsMultiline returns true, Enter is routed to the control
// instead of submitting the form.
type MultilineInput interface {
	IsMultiline() bool
}

// HelpTextProvider is an optional interface for FormControls that supply
// their own context-sensitive help line. When the focused control implements
// this interface, its text replaces the default form help line at the bottom
// of the screen.
type HelpTextProvider interface {
	HelpText() string
}

// ControlFactory creates a FormControl for a given field descriptor.
// The styles parameter carries the active Styles so custom controls can
// follow the application's colour scheme. Register factories via
// tui.WithCustomControl or tui.WithCustomControlForField.
type ControlFactory func(field protocli.TUIFieldDescriptor, styles Styles) FormControl

// KeyBind pairs a key hint with its action label for use in help text.
type KeyBind struct {
	Keys string // e.g. "↑↓/Tab", "Esc", "Enter"
	Op   string // e.g. "navigate", "back", "submit"
}

// FormHelpText builds a formatted help text string from the provided keybinds,
// always appending the global form keybind "Esc: back" at the end.
// Use this in HelpTextProvider implementations to stay consistent with the
// default form help line.
func FormHelpText(binds ...KeyBind) string {
	all := append(binds, KeyBind{"Esc", "back"})
	parts := make([]string, len(all))
	for i, b := range all {
		parts[i] = b.Keys + ": " + b.Op
	}
	return strings.Join(parts, "  •  ")
}

// ─── Text control ─────────────────────────────────────────────────────────────

// textControl is a plain text input used for string, enum, bytes, and WKT fields.
type textControl struct{ input textinput.Model }

// NewTextControl returns a single-line text FormControl.
func NewTextControl(placeholder, defaultValue string, styles Styles) FormControl {
	ti := textinput.New()
	ti.Placeholder = placeholder
	ti.PlaceholderStyle = lipgloss.NewStyle().Foreground(styles.Colors.Secondary)
	ti.Width = 60
	if defaultValue != "" {
		ti.SetValue(defaultValue)
	}
	return &textControl{input: ti}
}

func (c *textControl) View() string               { return c.input.View() }
func (c *textControl) Focus() tea.Cmd             { return c.input.Focus() }
func (c *textControl) Blur()                      { c.input.Blur() }
func (c *textControl) Value() string              { return c.input.Value() }
func (c *textControl) Update(msg tea.Msg) tea.Cmd {
	var cmd tea.Cmd
	c.input, cmd = c.input.Update(msg)
	return cmd
}

// ─── Number control ───────────────────────────────────────────────────────────

// numberControl is a text input restricted to numeric characters.
type numberControl struct {
	input   textinput.Model
	isFloat bool
}

// NewNumberControl returns a numeric text FormControl.
// When isFloat is true, a single decimal point is also accepted.
func NewNumberControl(placeholder, defaultValue string, isFloat bool, styles Styles) FormControl {
	ti := textinput.New()
	ti.Placeholder = placeholder
	ti.PlaceholderStyle = lipgloss.NewStyle().Foreground(styles.Colors.Secondary)
	ti.Width = 60
	if defaultValue != "" {
		ti.SetValue(defaultValue)
	}
	return &numberControl{input: ti, isFloat: isFloat}
}

func (c *numberControl) View() string   { return c.input.View() }
func (c *numberControl) Focus() tea.Cmd { return c.input.Focus() }
func (c *numberControl) Blur()          { c.input.Blur() }
func (c *numberControl) Value() string  { return c.input.Value() }
func (c *numberControl) Update(msg tea.Msg) tea.Cmd {
	if key, ok := msg.(tea.KeyMsg); ok && len(key.String()) == 1 {
		ch := rune(key.String()[0])
		cur := c.input.Value()
		switch {
		case ch >= '0' && ch <= '9':
			// always allowed
		case ch == '-':
			if len(cur) > 0 {
				return nil // only valid as the very first character
			}
		case ch == '.':
			if !c.isFloat || strings.Contains(cur, ".") {
				return nil
			}
		default:
			return nil // block all other printable characters
		}
	}
	var cmd tea.Cmd
	c.input, cmd = c.input.Update(msg)
	return cmd
}

// ─── Toggle control ───────────────────────────────────────────────────────────

// toggleControl is a boolean checkbox toggled with Space.
type toggleControl struct {
	checked    bool
	focused    bool
	onStyle    lipgloss.Style
	offStyle   lipgloss.Style
	focusStyle lipgloss.Style
}

// NewToggleControl returns a boolean toggle FormControl.
// defaultValue should be "true" or "false" (or empty for false).
func NewToggleControl(defaultValue string, styles Styles) FormControl {
	return &toggleControl{
		checked:    defaultValue == "true",
		onStyle:    styles.ToggleOn,
		offStyle:   styles.ToggleOff,
		focusStyle: styles.ToggleFocus,
	}
}

func (c *toggleControl) Focus() tea.Cmd { c.focused = true; return nil }
func (c *toggleControl) Blur()          { c.focused = false }
func (c *toggleControl) Value() string {
	if c.checked {
		return "true"
	}
	return "false"
}
func (c *toggleControl) View() string {
	box, label := "[ ]", " false"
	if c.checked {
		box, label = "[✓]", " true"
	}
	s := box + label
	switch {
	case c.focused:
		return c.focusStyle.Render(s)
	case c.checked:
		return c.onStyle.Render(s)
	default:
		return c.offStyle.Render(s)
	}
}
func (c *toggleControl) HelpText() string {
	return FormHelpText(
		KeyBind{"↑↓/Tab", "navigate"},
		KeyBind{"Space", "toggle"},
		KeyBind{"Enter", "submit"},
	)
}
func (c *toggleControl) Update(msg tea.Msg) tea.Cmd {
	if key, ok := msg.(tea.KeyMsg); ok && key.String() == " " {
		c.checked = !c.checked
	}
	return nil
}

// ─── List control ─────────────────────────────────────────────────────────────

// listControl collects repeated values. Typing a comma commits the current
// item; Backspace on an empty input removes the last confirmed item.
type listControl struct {
	items     []string
	current   textinput.Model
	itemStyle lipgloss.Style
}

// NewListControl returns a repeated-value FormControl.
// Comma separates items; Backspace on an empty input removes the last item.
func NewListControl(placeholder, defaultValue string, styles Styles) FormControl {
	ti := textinput.New()
	ti.Placeholder = placeholder
	ti.Width = 60
	lc := &listControl{current: ti, itemStyle: styles.ListItem}
	if defaultValue != "" {
		for _, item := range strings.Split(defaultValue, ",") {
			if v := strings.TrimSpace(item); v != "" {
				lc.items = append(lc.items, v)
			}
		}
	}
	return lc
}

func (c *listControl) Focus() tea.Cmd { return c.current.Focus() }
func (c *listControl) Blur()          { c.current.Blur() }
func (c *listControl) Value() string {
	all := append(append([]string{}, c.items...), strings.TrimSpace(c.current.Value()))
	var out []string
	for _, v := range all {
		if v != "" {
			out = append(out, v)
		}
	}
	return strings.Join(out, ",")
}
func (c *listControl) View() string {
	var sb strings.Builder
	for _, item := range c.items {
		sb.WriteString(c.itemStyle.Render("• "+item) + "\n")
	}
	sb.WriteString(c.current.View())
	return sb.String()
}
func (c *listControl) Update(msg tea.Msg) tea.Cmd {
	if key, ok := msg.(tea.KeyMsg); ok {
		switch key.String() {
		case ",":
			if val := strings.TrimSpace(c.current.Value()); val != "" {
				c.items = append(c.items, val)
				c.current.SetValue("")
			}
			return nil
		case "backspace":
			if c.current.Value() == "" && len(c.items) > 0 {
				c.items = c.items[:len(c.items)-1]
				return nil
			}
		}
	}
	var cmd tea.Cmd
	c.current, cmd = c.current.Update(msg)
	return cmd
}
