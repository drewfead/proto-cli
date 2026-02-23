// tuicli demonstrates the --interactive TUI mode for proto-cli.
// Run with --interactive to launch the bubbletea TUI.
//
// This example exercises the following TUI features:
//
//  1. WKT (Timestamp): ScheduledFarewell uses WithTimestampControl() to render
//     the send_at field as an interactive date+time picker instead of a text box.
//
//  2. Nested message flattening: ScheduledFarewell's DeliveryAddress sub-fields
//     appear as "Delivery Address › Street" / "Delivery Address › City".
//
//  3. Custom form control: ColoredGreet's RgbColor field uses a compact "R,G,B"
//     text input that applies values via FieldApplier. Registered via
//     tui.WithCustomControl using the proto message full name.
//
//  4. JSON editor: LeaveNote's metadata string field uses bubbles.NewJSONInput,
//     which wraps bubbles.NewVimInput with a JSON validator. Registered via
//     tui.WithCustomControlForField because it is a plain string field (not a
//     message type).
//
//  5. SystemTimezone picker: ScheduleCall's `when` field is a google.protobuf.Timestamp.
//     tui.WithCustomControlForField("when", ...) overrides tui.WithTimestampControl()
//     for this field specifically, using bubbles.SystemTimezone so the user enters
//     local time that is normalised to UTC for the Timestamp wire format.
//
//  6. Custom theme: tui.WithTheme sets a magenta colour scheme. All styles are
//     derived automatically from the theme's color, spacing, and border tokens.
//
//  7. Dispatched response views: a ResponseViewFactory closure selects the view
//     by method name. CountdownFarewell uses NewCardGridResponseView (one card
//     per streamed message); all other methods use NewTableResponseView.
//
//  8. Multi-binding card actions: CountdownFarewell uses WithCardAction with a
//     CardActionHandlerFunc (single "enter" binding) to make each card focusable.
//     Pressing Enter pops up a modal that renders the number in a large pixel-font
//     via bubbles.BigText. DirectoryService uses a named directoryCardAction with
//     ":g" (greet) and ":f" (farewell) bindings — multi-char sequences are buffered
//     by the grid and the pending prefix is shown in the help line.
//
//  9. Cross-service navigation: DirectoryService.ListPeople streams a contact
//     directory as a selectable card grid. :g navigates to Greeter › Say Hello and
//     :f navigates to Farewell › Bid Farewell, both with the recipient's name
//     pre-populated via tui.NavigateToForm.
package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	protocli "github.com/drewfead/proto-cli"
	"github.com/drewfead/proto-cli/contrib/tui"
	tuibubbles "github.com/drewfead/proto-cli/contrib/tui/bubbles"
	tui_example "github.com/drewfead/proto-cli/examples/tui"
	"google.golang.org/protobuf/proto"
)

func main() {
	greeter := tui_example.GreeterServiceCommand(context.Background(), &tui_example.GreeterServiceImpl{})
	farewell := tui_example.FarewellServiceCommand(context.Background(), &tui_example.FarewellServiceImpl{})
	directory := tui_example.DirectoryServiceCommand(context.Background(), &tui_example.DirectoryServiceImpl{})

	app, err := protocli.RootCommand("tuicli",
		protocli.Service(greeter),
		protocli.Service(farewell),
		protocli.Service(directory),
		protocli.WithInteractive(tui.New(
			// Retheme with a custom magenta theme. All styles are derived
			// automatically from the color tokens; spacing and borders use defaults.
			tui.WithTheme(tuibubbles.Theme{
				Colors: tuibubbles.ThemeColors{
					Primary:   lipgloss.Color("205"),
					Secondary: lipgloss.Color("241"),
					Accent:    lipgloss.Color("213"),
					Error:     lipgloss.Color("196"),
					Success:   lipgloss.Color("2"),
					Border:    lipgloss.Color("238"),
				},
			}),
			// Dispatch response views by method name:
			//  - list-people: selectable card grid; Enter navigates to the greeter
			//    form with the chosen contact's name pre-populated.
			//  - countdown-farewell: accumulating card grid; Enter opens a big-number modal.
			//  - all others: two-column table.
			tui.WithResponseView(func(desc protocli.TUIResponseDescriptor, styles tuibubbles.Styles) tuibubbles.ResponseView {
				if desc.MethodName == "list-people" {
					return tuibubbles.NewCardGridResponseView(
						// Single-column layout with cards that fill the terminal width.
						tuibubbles.WithColumns(1),
						tuibubbles.WithFillWidth(),
						// Render name as a bold header with bio below.
						tuibubbles.WithCardContent(func(msg proto.Message, innerWidth int, s tuibubbles.Styles) string {
							person, ok := msg.(*tui_example.PersonCard)
							if !ok {
								return fmt.Sprintf("%v", msg)
							}
							header := s.Title.Width(innerWidth).Render(person.Name)
							body := s.Subtitle.Width(innerWidth).Render(person.Bio)
							return lipgloss.JoinVertical(lipgloss.Left, header, body)
						}),
						tuibubbles.WithCardAction(directoryCardAction{}),
					)(desc, styles)
				}
				if desc.MethodName == "countdown-farewell" {
					capturedStyles := styles
					return tuibubbles.NewCardGridResponseView(
						// Make each card selectable: Enter pops up a modal that
						// renders the countdown number very large via BigText.
						tuibubbles.WithCardAction(tuibubbles.CardActionHandlerFunc{
							Bindings: []tuibubbles.CardKeyBinding{{Key: "enter", Description: "inspect"}},
							Fn: func(_ context.Context, _ string, msg proto.Message) tuibubbles.CardSelectResult {
								resp, ok := msg.(*tui_example.CountdownFarewellResponse)
								if !ok {
									return tuibubbles.CardSelectCmd(nil)
								}
								text := resp.Message
								// Strip the trailing "…" (U+2026) to isolate the number.
								number := strings.TrimSuffix(text, "…")
								number = strings.TrimSpace(number)
								var content string
								if _, err := strconv.Atoi(number); err == nil {
									// Pure number — render it with the big pixel font.
									content = tuibubbles.BigText(number, capturedStyles.Title.GetForeground())
								} else {
									// Final "Goodbye…" message — show it styled normally.
									content = capturedStyles.Title.Render(text)
								}
								return tuibubbles.CardSelectCmd(tui.ShowModal(text, content))
							},
						}),
					)(desc, styles)
				}
				return tuibubbles.NewTableResponseView()(desc, styles)
			}),
			// Render google.protobuf.Timestamp fields as an interactive date+time picker.
			tui.WithTimestampControl(),
			// Register a custom form control for RgbColor that renders a compact
			// "R,G,B" text input instead of three separate numeric fields.
			tui.WithCustomControl("tui_example.RgbColor", rgbColorControlFactory),
			// Render the plain-string "metadata" field as a freeform JSON editor.
			tui.WithCustomControlForField("metadata", func(field protocli.TUIFieldDescriptor, s tuibubbles.Styles) tuibubbles.FormControl {
				return tuibubbles.NewJSONInput("", field, s)
			}),
			// ScheduleCall: render "when" (a google.protobuf.Timestamp) as a
			// date-time picker that accepts local time (SystemTimezone). This
			// overrides WithTimestampControl() for this specific field — the picker
			// normalises to UTC for the Timestamp wire format (ToUTC default).
			tui.WithCustomControlForField("when", func(field protocli.TUIFieldDescriptor, s tuibubbles.Styles) tuibubbles.FormControl {
				return tuibubbles.NewDateTimeControl(field.DefaultValue, s,
					tuibubbles.WithInputTimezone(tuibubbles.SystemTimezone),
				)
			}),
		)),
	)
	if err != nil {
		log.Fatal(err)
	}

	if err := app.Run(context.Background(), os.Args); err != nil {
		log.Fatal(err)
	}
}

// directoryCardAction handles :g (greet) and :f (farewell) key sequences on
// PersonCard entries in the Browse Directory grid.
type directoryCardAction struct{}

func (directoryCardAction) KeyBindings() []tuibubbles.CardKeyBinding {
	return []tuibubbles.CardKeyBinding{
		{Key: ":g", Description: "greet"},
		{Key: ":f", Description: "farewell"},
	}
}

func (directoryCardAction) Handle(_ context.Context, key string, msg proto.Message) tuibubbles.CardSelectResult {
	person, ok := msg.(*tui_example.PersonCard)
	if !ok {
		return tuibubbles.CardSelectCmd(nil)
	}
	switch key {
	case ":g":
		return tuibubbles.CardSelectCmd(
			tui.NavigateToForm("greeter", "greet", map[string]string{"name": person.Name}),
		)
	case ":f":
		return tuibubbles.CardSelectCmd(
			tui.NavigateToForm("farewell", "farewell", map[string]string{"name": person.Name}),
		)
	}
	return tuibubbles.CardSelectCmd(nil)
}

// rgbColorControlFactory creates the custom FormControl for tui_example.RgbColor fields.
// It receives the active Styles so it can follow the app's colour scheme.
func rgbColorControlFactory(field protocli.TUIFieldDescriptor, styles tuibubbles.Styles) tuibubbles.FormControl {
	ti := textinput.New()
	ti.Placeholder = "R,G,B  (e.g. 255,128,0)"
	ti.Width = 60
	return &rgbColorControl{input: ti, field: field, styles: styles}
}

// rgbColorControl is a single-line "R,G,B" input that applies each channel to the
// proto message's sub-fields via the generated setters. It implements both
// tuibubbles.FormControl and tuibubbles.FieldApplier.
type rgbColorControl struct {
	input  textinput.Model
	field  protocli.TUIFieldDescriptor
	styles tuibubbles.Styles
}

func (c *rgbColorControl) View() string               { return c.input.View() }
func (c *rgbColorControl) Focus() tea.Cmd             { return c.input.Focus() }
func (c *rgbColorControl) Blur()                      { c.input.Blur() }
func (c *rgbColorControl) Value() string              { return c.input.Value() }
func (c *rgbColorControl) Update(msg tea.Msg) tea.Cmd {
	// Filter non-numeric input (digits, comma, spaces only).
	if key, ok := msg.(tea.KeyMsg); ok && len(key.String()) == 1 {
		ch := rune(key.String()[0])
		if !((ch >= '0' && ch <= '9') || ch == ',' || ch == ' ') {
			return nil
		}
	}
	var cmd tea.Cmd
	c.input, cmd = c.input.Update(msg)
	return cmd
}

// Apply implements tuibubbles.FieldApplier: parses "R,G,B" and calls each sub-field's setter.
func (c *rgbColorControl) Apply(msg proto.Message) error {
	raw := strings.TrimSpace(c.input.Value())
	if raw == "" {
		return nil // color is optional
	}

	parts := strings.SplitN(raw, ",", 3)
	if len(parts) != 3 {
		return fmt.Errorf("color must be in R,G,B format (e.g. 255,128,0), got %q", raw)
	}

	for i, part := range parts {
		val := strings.TrimSpace(part)
		n, err := strconv.ParseInt(val, 10, 32)
		if err != nil {
			return fmt.Errorf("invalid channel value %q: must be an integer 0-255", val)
		}
		if n < 0 || n > 255 {
			return fmt.Errorf("channel value %d is out of range (0-255)", n)
		}
		// c.field.Fields holds the R, G, B sub-descriptors in order.
		if i < len(c.field.Fields) && c.field.Fields[i].Setter != nil {
			if err := c.field.Fields[i].Setter(msg, val); err != nil {
				return err
			}
		}
	}
	return nil
}
