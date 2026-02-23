// Package tui provides a bubbletea-based interactive TUI for proto-cli.
// It implements the protocli.TUIProvider interface, allowing users to browse
// and invoke gRPC methods through a terminal UI.
//
// Usage:
//
//	app, err := protocli.RootCommand("myapp",
//	    protocli.Service(svc),
//	    protocli.WithInteractive(tui.New()),
//	)
//
// Theming
//
// The quickest way to retheme the TUI is WithTheme, which accepts a Theme value
// controlling colors, spacing tokens, border shapes, and optional custom styles.
// All lipgloss styles are derived automatically:
//
//	tui.New(tui.WithTheme(bubbles.Theme{
//	    Colors: bubbles.ThemeColors{
//	        Primary:   lipgloss.Color("205"),
//	        Secondary: lipgloss.Color("241"),
//	        Accent:    lipgloss.Color("213"),
//	        Error:     lipgloss.Color("196"),
//	        Success:   lipgloss.Color("2"),
//	        Border:    lipgloss.Color("238"),
//	    },
//	}))
//
// Spacing and border shapes can also be customised through the theme:
//
//	theme := bubbles.DefaultTheme()
//	theme.Spacing.SM = 3                             // wider tab padding
//	theme.Border.Control = lipgloss.NormalBorder()   // square control borders
//	tui.New(tui.WithTheme(theme))
//
// Register custom styles in Theme.Custom and retrieve them in your own bubbles
// via Styles.Get:
//
//	theme := bubbles.DefaultTheme()
//	theme.Custom = map[string]lipgloss.Style{
//	    "badge": lipgloss.NewStyle().Bold(true).Background(lipgloss.Color("57")),
//	}
//	tui.New(tui.WithTheme(theme))
//
//	// Inside a custom FormControl or ResponseView:
//	badge := styles.Get("badge").Render("NEW")
//
// For fine-grained per-field adjustments on top of a theme, use WithStyleOverride:
//
//	tui.New(
//	    tui.WithTheme(myTheme),
//	    tui.WithStyleOverride(func(s *bubbles.Styles) {
//	        s.FocusedControl = s.FocusedControl.MaxWidth(80)
//	    }),
//	)
//
// For complete control, replace the full style set with WithStyles:
//
//	s := bubbles.DefaultStyles()
//	s.Title = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("205"))
//	tui.New(tui.WithStyles(s))
//
// Custom response views
//
// Register a ResponseViewFactory via WithResponseView to replace the default
// JSON viewport with any bubbletea sub-model:
//
//	tui.New(tui.WithResponseView(bubbles.NewTableResponseView()))
package tui

import (
	"context"
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/urfave/cli/v3"
	"google.golang.org/protobuf/proto"

	bubbles "github.com/drewfead/proto-cli/contrib/tui/bubbles"
	protocli "github.com/drewfead/proto-cli"
)

// ShowModalMsg is a bubbletea message that instructs the root model to display
// a modal overlay on top of the current screen. Return it from a
// bubbles.CardSelectHandler to pop up a modal when a card is selected:
//
//	bubbles.WithOnSelect(func(msg proto.Message) tea.Cmd {
//	    return tui.ShowModal("Selected", renderContent(msg))
//	})
type ShowModalMsg struct {
	// Title is rendered at the top of the modal box. May be empty.
	Title string
	// Content is the body of the modal box.
	Content string
}

// ShowModal returns a tea.Cmd that triggers a modal overlay with the given
// title and content. Pass it as the return value of a bubbles.CardSelectHandler.
func ShowModal(title, content string) tea.Cmd {
	return func() tea.Msg { return ShowModalMsg{Title: title, Content: content} }
}

// NavigateToFormMsg instructs the root model to navigate to a specific method's
// form screen on a given service, optionally pre-populating named fields.
// Return it from a CardSelectHandler via CardSelectCmd to jump the user to a
// form after selecting a card — for example, to pre-fill a recipient name.
type NavigateToFormMsg struct {
	// ServiceName is the TUI name of the target service (e.g. "greeter").
	ServiceName string
	// MethodName is the TUI name of the target method (e.g. "greet").
	MethodName string
	// FieldValues pre-populates specific form fields keyed by field flag-name.
	// Values override the proto-defined default for the matching field.
	FieldValues map[string]string
}

// NavigateToForm returns a tea.Cmd that navigates the TUI to the named
// service/method form, pre-filling any fields specified in fieldValues.
// Use it from a bubbles.CardSelectHandler to jump context after a card selection:
//
//	bubbles.WithOnSelect(func(ctx context.Context, msg proto.Message) bubbles.CardSelectResult {
//	    person := msg.(*pb.PersonCard)
//	    return bubbles.CardSelectCmd(tui.NavigateToForm("greeter", "greet", map[string]string{
//	        "name": person.Name,
//	    }))
//	})
func NavigateToForm(serviceName, methodName string, fieldValues map[string]string) tea.Cmd {
	return func() tea.Msg {
		return NavigateToFormMsg{ServiceName: serviceName, MethodName: methodName, FieldValues: fieldValues}
	}
}

// streamItemMsg carries a single message received from a server-streaming RPC.
type streamItemMsg struct{ msg proto.Message }

// streamDoneMsg signals that a server-streaming RPC has ended.
// err is nil on clean completion or context cancellation; non-nil on error.
type streamDoneMsg struct{ err error }

// readStreamCh returns a tea.Cmd that blocks until the next value is
// available on ch and delivers it as a tea.Msg. Call it again from Update
// after each streamItemMsg to keep polling the stream.
func readStreamCh(ch <-chan tea.Msg) tea.Cmd {
	return func() tea.Msg { return <-ch }
}

// Option configures the TUI provider.
type Option func(*provider)

// WithStyles replaces the provider's full style set. Obtain the defaults via
// bubbles.DefaultStyles, modify the fields you care about, then pass the result here.
func WithStyles(styles bubbles.Styles) Option {
	return func(p *provider) {
		p.styles = styles
	}
}

// WithTheme rebuilds the full style set from the given theme.
// The theme controls colors, spacing tokens, border shapes, and any custom
// styles forwarded to Styles.Custom. Use WithStyleOverride afterwards to tweak
// any individual style that the theme does not capture.
func WithTheme(theme bubbles.Theme) Option {
	return func(p *provider) {
		p.styles = bubbles.StylesFromTheme(theme)
	}
}

// WithStyleOverride applies a modifier function to the current style set in place.
// Use this to adjust individual styles after WithPalette (or without replacing the
// entire set via WithStyles). Options are applied in order, so WithPalette followed
// by WithStyleOverride works as expected.
func WithStyleOverride(fn func(*bubbles.Styles)) Option {
	return func(p *provider) {
		fn(&p.styles)
	}
}

// WithCustomControl registers a custom FormControl factory for a given proto
// message full name (e.g. "google.protobuf.Timestamp" or "mypackage.MyType").
// The factory is called in newFormModel when a TUIFieldKindMessage field with
// a matching MessageFullName is encountered.
func WithCustomControl(messageFullName string, factory bubbles.ControlFactory) Option {
	return func(p *provider) {
		if p.customControls == nil {
			p.customControls = make(map[string]bubbles.ControlFactory)
		}
		p.customControls[messageFullName] = factory
	}
}

// WithCustomControlForField registers a custom FormControl factory for a
// specific field identified by its flag name (e.g. "metadata", "when").
// This works for any field kind — use it when you want to override a scalar,
// repeated, or message field's default control for a specific field.
// Name-based registrations take priority over type-based registrations
// (WithCustomControl / WithTimestampControl), so you can use this to give a
// particular field different behaviour from the global type-based default.
func WithCustomControlForField(fieldName string, factory bubbles.ControlFactory) Option {
	return func(p *provider) {
		if p.customControlsByName == nil {
			p.customControlsByName = make(map[string]bubbles.ControlFactory)
		}
		p.customControlsByName[fieldName] = factory
	}
}

// WithTimestampControl registers an interactive date+time picker as the custom
// form control for google.protobuf.Timestamp fields. Use this instead of the
// default RFC3339 text input when you want a segment-by-segment picker UI.
//
// Pass DateTimeControlOption values to configure timezone behaviour:
//
//	tui.WithTimestampControl(
//	    bubbles.WithInputTimezone(bubbles.UserProvidedTimezone),
//	    bubbles.WithTZNormalization(bubbles.RetainSourceTimezone),
//	)
func WithTimestampControl(opts ...bubbles.DateTimeControlOption) Option {
	return WithCustomControl("google.protobuf.Timestamp", func(field protocli.TUIFieldDescriptor, styles bubbles.Styles) bubbles.FormControl {
		return bubbles.NewDateTimeControl(field.DefaultValue, styles, opts...)
	})
}

// WithResponseView registers a factory that creates a custom ResponseView for
// displaying RPC responses. The default implementation marshals the proto to
// indented JSON and presents it in a scrollable viewport.
func WithResponseView(factory bubbles.ResponseViewFactory) Option {
	return func(p *provider) {
		p.responseViewFactory = factory
	}
}

// provider implements protocli.TUIProvider.
type provider struct {
	styles               bubbles.Styles
	customControls       map[string]bubbles.ControlFactory // keyed by proto message full name
	customControlsByName map[string]bubbles.ControlFactory // keyed by field flag name
	responseViewFactory  bubbles.ResponseViewFactory       // nil = default JSON viewport
}

// New creates a new TUI provider. Default styles are applied before any options.
func New(opts ...Option) protocli.TUIProvider {
	p := &provider{styles: bubbles.DefaultStyles()}
	for _, opt := range opts {
		opt(p)
	}
	return p
}

// Run launches the interactive TUI.
func (p *provider) Run(ctx context.Context, cmd *cli.Command, services []protocli.TUIService, opts ...protocli.TUIRunOption) error {
	cfg := protocli.TUIRunConfig{}
	for _, opt := range opts {
		opt(&cfg)
	}
	m := newRootModel(ctx, cmd, services, p.styles, p.customControls, p.customControlsByName, p.responseViewFactory, cfg)
	prog := tea.NewProgram(m, tea.WithAltScreen())
	_, err := prog.Run()
	return err
}

// screen identifies which screen is currently displayed.
type screen int

const (
	screenMethodList screen = iota
	screenForm
	screenResponse
)

// rootModel is the top-level bubbletea model.
type rootModel struct {
	ctx      context.Context
	cmd      *cli.Command
	services []protocli.TUIService

	currentScreen   screen
	selectedService int
	selectedMethod  int

	// Sub-models
	methodList   list.Model
	form         formModel
	responseView bubbles.ResponseView // nil until first invocation

	errorText string

	// Streaming state. Active only while a server-streaming RPC is in flight.
	streamCh     <-chan tea.Msg  // nil when no stream is active
	streamActive bool
	streamCount  int
	streamCancel context.CancelFunc // cancels the stream's derived context
	streamBuf    []proto.Message    // fallback buffer for non-StreamingResponseView factories

	width  int
	height int

	// Modal overlay state. Active when the user selects a card with WithOnSelect.
	modalActive  bool
	modalTitle   string
	modalContent string

	styles               bubbles.Styles
	customControls       map[string]bubbles.ControlFactory
	customControlsByName map[string]bubbles.ControlFactory
	responseViewFactory  bubbles.ResponseViewFactory // nil = default JSON viewport
}

// methodItem implements list.Item for method descriptors.
type methodItem struct {
	method protocli.TUIMethod
}

func (i methodItem) Title() string       { return i.method.TUIDisplayName() }
func (i methodItem) Description() string { return i.method.TUIDescription() }
func (i methodItem) FilterValue() string { return i.method.TUIName() }

func newRootModel(
	ctx context.Context,
	cmd *cli.Command,
	services []protocli.TUIService,
	styles bubbles.Styles,
	customControls map[string]bubbles.ControlFactory,
	customControlsByName map[string]bubbles.ControlFactory,
	responseViewFactory bubbles.ResponseViewFactory,
	cfg protocli.TUIRunConfig,
) rootModel {
	m := rootModel{
		ctx:                  ctx,
		cmd:                  cmd,
		services:             services,
		currentScreen:        screenMethodList,
		selectedService:      0,
		styles:               styles,
		customControls:       customControls,
		customControlsByName: customControlsByName,
		responseViewFactory:  responseViewFactory,
	}

	// Apply deep-link: find the requested starting service.
	if cfg.StartServiceName != "" {
		for i, svc := range services {
			if svc.TUIName() == cfg.StartServiceName {
				m.selectedService = i
				break
			}
		}
	}

	if len(services) > 0 {
		m.methodList = newMethodList(services[m.selectedService])
	}

	// Apply deep-link: if a method was also requested, open its form directly.
	if cfg.StartMethodName != "" && m.selectedService < len(services) {
		for i, method := range services[m.selectedService].TUIMethods() {
			if method.TUIName() == cfg.StartMethodName && !method.TUIHidden() {
				m.selectedMethod = i
				m.form = newFormModel(method, styles, customControls, customControlsByName, cfg.FieldValues)
				m.currentScreen = screenForm
				break
			}
		}
	}

	return m
}

func newMethodList(svc protocli.TUIService) list.Model {
	items := make([]list.Item, 0, len(svc.TUIMethods()))
	for _, m := range svc.TUIMethods() {
		if !m.TUIHidden() {
			items = append(items, methodItem{method: m})
		}
	}
	l := list.New(items, list.NewDefaultDelegate(), 60, 20)
	l.SetShowTitle(false)
	l.SetShowStatusBar(false)
	l.SetFilteringEnabled(true)
	return l
}

// tabBarHeight returns the number of lines consumed by the tab bar,
// or 0 if there is only one service (no tabs rendered).
func (m rootModel) tabBarHeight() int {
	if len(m.services) > 1 {
		return 2 // tab row + separator line
	}
	return 0
}

// breadcrumbHeight returns the number of lines consumed by the method
// breadcrumb block (name + optional description + separator).
func (m rootModel) breadcrumbHeight(method protocli.TUIMethod) int {
	if method == nil {
		return 0
	}
	h := 2 // name + separator
	if method.TUIDescription() != "" {
		h++ // description line
	}
	return h
}

// headerHeight returns the total number of lines consumed by the shared
// top-of-screen header (tabs + breadcrumb).
func (m rootModel) headerHeight(method protocli.TUIMethod) int {
	return m.tabBarHeight() + m.breadcrumbHeight(method)
}

// viewHeader renders the consistent top-of-screen area: the service tab bar
// (when multiple services are present) followed by an optional method
// breadcrumb. All screens call this to anchor the same north-edge structure.
func (m rootModel) viewHeader(method protocli.TUIMethod) string {
	var sb strings.Builder

	if len(m.services) > 1 {
		sb.WriteString(m.viewTabs())
		sb.WriteString("\n")
	}

	if method != nil {
		indent := strings.Repeat(" ", m.styles.FormIndent)
		sb.WriteString(indent + m.styles.Title.Render(method.TUIDisplayName()) + "\n")
		if method.TUIDescription() != "" {
			sb.WriteString(indent + m.styles.Subtitle.Render(method.TUIDescription()) + "\n")
		}
		sb.WriteString(strings.Repeat("─", m.width) + "\n")
	}

	return sb.String()
}

func (m rootModel) Init() tea.Cmd {
	return nil
}

func (m rootModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case ShowModalMsg:
		m.modalActive = true
		m.modalTitle = msg.Title
		m.modalContent = msg.Content
		return m, nil

	case NavigateToFormMsg:
		for i, svc := range m.services {
			if svc.TUIName() != msg.ServiceName {
				continue
			}
			m.selectedService = i
			m.methodList = newMethodList(svc)
			m.methodList.SetSize(m.width, m.height-m.tabBarHeight())
			for j, method := range svc.TUIMethods() {
				if method.TUIName() == msg.MethodName && !method.TUIHidden() {
					m.selectedMethod = j
					m.form = newFormModel(method, m.styles, m.customControls, m.customControlsByName, msg.FieldValues)
					m.currentScreen = screenForm
					m.errorText = ""
					break
				}
			}
			break
		}
		return m, nil

	case bubbles.CardActionErrorMsg:
		m.errorText = msg.Err.Error()
		return m, nil

	case streamItemMsg:
		m.streamCount++
		if srv, ok := m.responseView.(bubbles.StreamingResponseView); ok {
			srv.Append(msg.msg)
		} else {
			m.streamBuf = append(m.streamBuf, msg.msg)
		}
		return m, readStreamCh(m.streamCh)

	case streamDoneMsg:
		m.streamActive = false
		m.streamCh = nil
		m.streamCancel = nil
		if srv, ok := m.responseView.(bubbles.StreamingResponseView); ok {
			srv.Done(msg.err)
		} else {
			// Fallback for views that don't implement StreamingResponseView:
			// show the last buffered message when the stream ends.
			if msg.err != nil {
				m.errorText = msg.err.Error()
			} else if len(m.streamBuf) > 0 {
				method := m.services[m.selectedService].TUIMethods()[m.selectedMethod]
				respViewHeight := m.height - m.headerHeight(method) - 2
				m.responseView.Init(m.ctx, m.streamBuf[len(m.streamBuf)-1], m.width, respViewHeight)
			}
			m.streamBuf = nil
		}
		return m, nil

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.methodList.SetSize(msg.Width, msg.Height-m.tabBarHeight())
		if m.currentScreen == screenResponse && m.responseView != nil {
			method := m.services[m.selectedService].TUIMethods()[m.selectedMethod]
			m.responseView.SetSize(msg.Width, msg.Height-m.headerHeight(method)-2)
		}

	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c":
			return m, tea.Quit

		case "esc":
			if m.modalActive {
				m.modalActive = false
				return m, nil
			}
			switch m.currentScreen {
			case screenMethodList:
				return m, tea.Quit
			case screenForm:
				// If the focused control claims Esc (e.g. JSON editor leaving
				// insert mode), delegate it instead of navigating back.
				if m.form.focused < len(m.form.controls) {
					if kc, ok := m.form.controls[m.form.focused].(bubbles.KeyCapturer); ok && kc.CapturesKey("esc") {
						var cmd tea.Cmd
						m.form, cmd = m.form.update(msg)
						return m, cmd
					}
				}
				m.currentScreen = screenMethodList
				m.errorText = ""
				return m, nil
			case screenResponse:
				if m.streamActive {
					if m.streamCancel != nil {
						m.streamCancel()
					}
					m.streamActive = false
					m.streamCh = nil
					m.streamBuf = nil
					m.streamCancel = nil
				}
				m.currentScreen = screenForm
				m.errorText = ""
				return m, nil
			}

		case "left":
			if m.currentScreen == screenMethodList && m.methodList.FilterState() != list.Filtering {
				m.selectedService = (m.selectedService - 1 + len(m.services)) % len(m.services)
				m.methodList = newMethodList(m.services[m.selectedService])
				m.methodList.SetSize(m.width, m.height-m.tabBarHeight())
				return m, nil
			}

		case "right":
			if m.currentScreen == screenMethodList && m.methodList.FilterState() != list.Filtering {
				m.selectedService = (m.selectedService + 1) % len(m.services)
				m.methodList = newMethodList(m.services[m.selectedService])
				m.methodList.SetSize(m.width, m.height-m.tabBarHeight())
				return m, nil
			}

		case "enter":
			switch m.currentScreen {
			case screenMethodList:
				if selected, ok := m.methodList.SelectedItem().(methodItem); ok {
					for i, method := range m.services[m.selectedService].TUIMethods() {
						if method == selected.method {
							m.selectedMethod = i
							break
						}
					}
					m.form = newFormModel(selected.method, m.styles, m.customControls, m.customControlsByName, nil)
					m.currentScreen = screenForm
					m.errorText = ""
				}
				return m, nil

			case screenForm:
				// Delegate Enter to multi-line controls so they can insert newlines.
				if m.form.focused < len(m.form.controls) {
					if ml, ok := m.form.controls[m.form.focused].(bubbles.MultilineInput); ok && ml.IsMultiline() {
						var cmd tea.Cmd
						m.form, cmd = m.form.update(msg)
						return m, cmd
					}
				}
				return m.submitForm()

			case screenResponse:
				if !m.streamActive {
					// If the response view claims Enter (e.g. card grid selection),
					// delegate to it instead of navigating back to the method list.
					if m.responseView != nil {
						if kc, ok := m.responseView.(bubbles.KeyCapturer); ok && kc.CapturesKey("enter") {
							cmd := m.responseView.Update(msg)
							return m, cmd
						}
					}
					m.currentScreen = screenMethodList
					return m, nil
				}
			}
		}

	}

	// Delegate to sub-models
	var cmd tea.Cmd
	switch m.currentScreen {
	case screenMethodList:
		m.methodList, cmd = m.methodList.Update(msg)
	case screenForm:
		m.form, cmd = m.form.update(msg)
	case screenResponse:
		if m.responseView != nil {
			cmd = m.responseView.Update(msg)
		}
	}

	return m, cmd
}

func (m rootModel) View() string {
	if m.modalActive {
		return m.viewModal()
	}
	switch m.currentScreen {
	case screenMethodList:
		return m.viewMethodList()
	case screenForm:
		return m.viewForm()
	case screenResponse:
		return m.viewResponse()
	}
	return ""
}

// viewModal renders a centered modal overlay that fills the terminal.
// The modal box contains an optional title, a body (e.g. BigText output),
// and a dimmed "Esc: dismiss" hint below the box.
func (m rootModel) viewModal() string {
	primaryColor := m.styles.Colors.Primary

	var bodyParts []string
	if m.modalTitle != "" {
		bodyParts = append(bodyParts, m.styles.Title.Render(m.modalTitle))
	}
	if m.modalContent != "" {
		if len(bodyParts) > 0 {
			bodyParts = append(bodyParts, "")
		}
		bodyParts = append(bodyParts, m.modalContent)
	}
	body := strings.Join(bodyParts, "\n")

	box := m.styles.Modal.
		BorderForeground(primaryColor).
		Render(body)

	boxWidth := lipgloss.Width(box)
	helpLine := lipgloss.NewStyle().
		Width(boxWidth).
		Align(lipgloss.Center).
		Render(m.styles.Help.Render("Esc: dismiss"))

	modal := lipgloss.JoinVertical(lipgloss.Center, box, helpLine)

	return lipgloss.Place(m.width, m.height,
		lipgloss.Center, lipgloss.Center,
		modal,
		lipgloss.WithWhitespaceBackground(m.styles.Colors.Background),
	)
}

func (m rootModel) viewTabs() string {
	tabs := make([]string, len(m.services))
	for i, svc := range m.services {
		if i == m.selectedService {
			tabs[i] = m.styles.ActiveTab.Render(svc.TUIDisplayName())
		} else {
			tabs[i] = m.styles.InactiveTab.Render(svc.TUIDisplayName())
		}
	}
	tabRow := lipgloss.JoinHorizontal(lipgloss.Top, tabs...)
	separator := strings.Repeat("─", m.width)
	return tabRow + "\n" + separator
}

func (m rootModel) viewMethodList() string {
	return m.viewHeader(nil) + m.methodList.View()
}

func (m rootModel) viewForm() string {
	method := m.services[m.selectedService].TUIMethods()[m.selectedMethod]
	var sb strings.Builder

	sb.WriteString(m.viewHeader(method))

	// Inner width of each field box: terminal width minus the left indent and
	// the full horizontal frame of the control box (border + padding both sides).
	// Floor at 60 so controls are never narrower than their natural width.
	innerWidth := m.width - m.styles.FormIndent - m.styles.FocusedControl.GetHorizontalFrameSize()
	if innerWidth < 60 {
		innerWidth = 60
	}
	indent := strings.Repeat(" ", m.styles.FormIndent)

	for i, ctrl := range m.form.controls {
		field := m.form.fields[i]

		// Label row
		label := m.styles.FieldLabel.Render(field.Label)
		if field.Required {
			label += m.styles.Required.Render(" *")
		}
		sb.WriteString(indent + label + "\n")

		// Optional usage / description
		if field.Usage != "" {
			sb.WriteString(indent + m.styles.Subtitle.Render(field.Usage) + "\n")
		}

		// Control wrapped in a rounded border box; focused field gets an accent border.
		var box lipgloss.Style
		if i == m.form.focused {
			box = m.styles.FocusedControl.Width(innerWidth)
		} else {
			box = m.styles.Control.Width(innerWidth)
		}
		controlView := strings.TrimRight(ctrl.View(), "\n")
		for _, line := range strings.Split(strings.TrimRight(box.Render(controlView), "\n"), "\n") {
			sb.WriteString(indent + line + "\n")
		}
		sb.WriteString("\n")
	}

	if m.errorText != "" {
		sb.WriteString(m.styles.Error.Render("Error: "+m.errorText) + "\n\n")
	}

	helpText := bubbles.FormHelpText(
		bubbles.KeyBind{"↑↓/Tab", "navigate"},
		bubbles.KeyBind{"Enter", "submit"},
	)
	if m.form.focused < len(m.form.controls) {
		ctrl := m.form.controls[m.form.focused]
		if htp, ok := ctrl.(bubbles.HelpTextProvider); ok {
			helpText = htp.HelpText()
		}
	}
	sb.WriteString(m.styles.Help.Render(helpText))

	return sb.String()
}

func (m rootModel) viewResponse() string {
	method := m.services[m.selectedService].TUIMethods()[m.selectedMethod]
	var sb strings.Builder

	sb.WriteString(m.viewHeader(method))

	if m.errorText != "" {
		sb.WriteString(m.styles.Error.Render("Error: "+m.errorText) + "\n\n")
	}
	if m.responseView != nil {
		sb.WriteString(m.responseView.View())
	}
	var helpText string
	if m.streamActive {
		helpText = fmt.Sprintf("Streaming… (%d received) • Esc: cancel", m.streamCount)
	} else {
		helpText = "Esc: back to form • Enter: back to methods"
		if m.responseView != nil {
			if htp, ok := m.responseView.(bubbles.HelpTextProvider); ok {
				if t := htp.HelpText(); t != "" {
					helpText = t
				}
			}
		}
	}
	sb.WriteString("\n" + m.styles.Help.Render(helpText))
	return sb.String()
}

func (m rootModel) submitForm() (tea.Model, tea.Cmd) {
	method := m.services[m.selectedService].TUIMethods()[m.selectedMethod]
	req := method.TUINewRequest()

	// Apply field values
	for i, ctrl := range m.form.controls {
		field := m.form.fields[i]

		// Controls that implement FieldApplier bypass the string-based setter path.
		if applier, ok := ctrl.(bubbles.FieldApplier); ok {
			if err := applier.Apply(req); err != nil {
				m.errorText = err.Error()
				return m, nil
			}
			continue
		}

		val := ctrl.Value()
		if val == "" {
			if field.Required {
				m.errorText = fmt.Sprintf("field %q is required", field.Label)
				return m, nil
			}
			continue
		}

		var err error
		if field.Appender != nil {
			// For repeated fields, split by comma
			for _, elem := range strings.Split(val, ",") {
				elem = strings.TrimSpace(elem)
				if elem == "" {
					continue
				}
				if err = field.Appender(req, elem); err != nil {
					break
				}
			}
		} else if field.Setter != nil {
			err = field.Setter(req, val)
		}
		if err != nil {
			m.errorText = err.Error()
			return m, nil
		}
	}

	fac := m.responseViewFactory
	if fac == nil {
		fac = bubbles.NewViewportResponseView()
	}
	rv := fac(method.TUIResponseDescriptor(), m.styles)
	respViewHeight := m.height - m.headerHeight(method) - 2

	if method.TUIIsStreaming() {
		// Derive a cancellable context so Esc can abort the stream.
		streamCtx, cancel := context.WithCancel(m.ctx)

		// Buffered so the goroutine can finish writing even if the user
		// navigates away before all messages are consumed.
		ch := make(chan tea.Msg, 16)
		go func() {
			err := method.TUIInvokeStream(streamCtx, m.cmd, req, func(msg proto.Message) error {
				ch <- streamItemMsg{msg: msg}
				return nil
			})
			ch <- streamDoneMsg{err: err}
		}()

		rv.Init(streamCtx, nil, m.width, respViewHeight)
		m.responseView = rv
		m.streamCh = ch
		m.streamActive = true
		m.streamCount = 0
		m.streamCancel = cancel
		m.streamBuf = nil
		m.currentScreen = screenResponse
		m.errorText = ""
		return m, readStreamCh(ch)
	}

	// Unary method
	resp, err := method.TUIInvoke(m.ctx, m.cmd, req)

	if err != nil {
		m.errorText = err.Error()
		rv.Init(m.ctx, nil, m.width, respViewHeight)
		m.responseView = rv
		m.currentScreen = screenResponse
		return m, nil
	}

	cmd := rv.Init(m.ctx, resp, m.width, respViewHeight)
	m.responseView = rv
	m.currentScreen = screenResponse
	m.errorText = ""
	return m, cmd
}

// ─── Form model ───────────────────────────────────────────────────────────────

// formModel holds state for the request form screen.
type formModel struct {
	method   protocli.TUIMethod
	fields   []protocli.TUIFieldDescriptor // only visible, non-hidden fields
	controls []bubbles.FormControl
	focused  int
}

// flattenFields recursively expands TUIFieldKindMessage fields into their sub-fields,
// prepending a breadcrumb label prefix so the user sees "Address › Street" etc.
// Message fields with a registered custom control are kept as-is (not expanded).
func flattenFields(fields []protocli.TUIFieldDescriptor, prefix string, customControls map[string]bubbles.ControlFactory) []protocli.TUIFieldDescriptor {
	var result []protocli.TUIFieldDescriptor
	for _, field := range fields {
		if field.Hidden {
			continue
		}
		if field.Kind == protocli.TUIFieldKindMessage {
			// A custom control registered for this message type handles it as a unit.
			if _, hasCustom := customControls[field.MessageFullName]; hasCustom {
				if prefix != "" {
					field.Label = prefix + " › " + field.Label
				}
				result = append(result, field)
				continue
			}
			// No custom control: recurse into the nested sub-fields.
			subPrefix := field.Label
			if prefix != "" {
				subPrefix = prefix + " › " + field.Label
			}
			result = append(result, flattenFields(field.Fields, subPrefix, customControls)...)
		} else {
			if prefix != "" {
				field.Label = prefix + " › " + field.Label
			}
			result = append(result, field)
		}
	}
	return result
}

// newFormModel builds a form for the given method. prefill optionally overrides
// the initial value of specific fields, keyed by field flag-name (e.g. "name").
// Pass nil for no pre-population (the normal case).
func newFormModel(method protocli.TUIMethod, styles bubbles.Styles, customControls map[string]bubbles.ControlFactory, customControlsByName map[string]bubbles.ControlFactory, prefill map[string]string) formModel {
	fm := formModel{method: method}

	flatFields := flattenFields(method.TUIInputFields(), "", customControls)

	for _, field := range flatFields {
		if field.Hidden {
			continue
		}

		// Prefill overrides the proto-defined default for this field.
		// Mutate the local copy so both the default control constructors and
		// custom control factories see the same effective starting value.
		if v, ok := prefill[field.Name]; ok {
			field.DefaultValue = v
		}

		var ctrl bubbles.FormControl

		// Name-based override has highest priority — checked before type-based so
		// WithCustomControlForField("when", ...) can override WithTimestampControl().
		if factory, ok := customControlsByName[field.Name]; ok {
			ctrl = factory(field, styles)
		}

		// Type-based custom control by proto message full name. Covers both ordinary
		// TUIFieldKindMessage fields and WKT fields (e.g. google.protobuf.Timestamp)
		// which the generator promotes to TUIFieldKindString but still annotates with
		// MessageFullName, allowing WithTimestampControl() to target them.
		if ctrl == nil && field.MessageFullName != "" {
			if factory, ok := customControls[field.MessageFullName]; ok {
				ctrl = factory(field, styles)
			}
		}

		// TUIFieldKindMessage fields only reach here when flattenFields kept them
		// because a custom control was registered. Guard against the unexpected case.
		if field.Kind == protocli.TUIFieldKindMessage && ctrl == nil {
			continue
		}

		if ctrl == nil {
			switch field.Kind {
			case protocli.TUIFieldKindBool:
				ctrl = bubbles.NewToggleControl(field.DefaultValue, styles)
			case protocli.TUIFieldKindInt:
				ctrl = bubbles.NewNumberControl(field.Usage, field.DefaultValue, false, styles)
			case protocli.TUIFieldKindFloat:
				ctrl = bubbles.NewNumberControl(field.Usage, field.DefaultValue, true, styles)
			case protocli.TUIFieldKindRepeated:
				if field.Appender == nil {
					// Repeated messages without a custom control cannot be handled.
					continue
				}
				ctrl = bubbles.NewListControl("item, item, …", field.DefaultValue, styles)
			case protocli.TUIFieldKindEnum:
				placeholder := field.Usage
				if len(field.EnumValues) > 0 {
					names := make([]string, 0, len(field.EnumValues))
					for _, ev := range field.EnumValues {
						names = append(names, ev.Name)
					}
					placeholder = strings.Join(names, "|")
				}
				ctrl = bubbles.NewTextControl(placeholder, field.DefaultValue, styles)
			default:
				ctrl = bubbles.NewTextControl(field.Usage, field.DefaultValue, styles)
			}
		}

		fm.fields = append(fm.fields, field)
		fm.controls = append(fm.controls, ctrl)
	}

	if len(fm.controls) > 0 {
		fm.controls[0].Focus()
	}

	return fm
}

func (fm formModel) nextField() (formModel, tea.Cmd) {
	if len(fm.controls) == 0 {
		return fm, nil
	}
	fm.controls[fm.focused].Blur()
	fm.focused = (fm.focused + 1) % len(fm.controls)
	return fm, fm.controls[fm.focused].Focus()
}

func (fm formModel) prevField() (formModel, tea.Cmd) {
	if len(fm.controls) == 0 {
		return fm, nil
	}
	fm.controls[fm.focused].Blur()
	fm.focused = (fm.focused - 1 + len(fm.controls)) % len(fm.controls)
	return fm, fm.controls[fm.focused].Focus()
}

func (fm formModel) update(msg tea.Msg) (formModel, tea.Cmd) {
	if key, ok := msg.(tea.KeyMsg); ok {
		// Let controls that claim navigation keys handle them directly.
		if fm.focused < len(fm.controls) {
			if kc, ok2 := fm.controls[fm.focused].(bubbles.KeyCapturer); ok2 && kc.CapturesKey(key.String()) {
				cmd := fm.controls[fm.focused].Update(msg)
				return fm, cmd
			}
		}
		switch key.String() {
		case "down", "tab":
			return fm.nextField()
		case "up", "shift+tab":
			return fm.prevField()
		}
	}
	if fm.focused < len(fm.controls) {
		cmd := fm.controls[fm.focused].Update(msg)
		return fm, cmd
	}
	return fm, nil
}
