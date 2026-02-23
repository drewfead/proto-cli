package bubbles

import "github.com/charmbracelet/lipgloss"

// Styles holds all lipgloss styles used by the TUI. Obtain a baseline via
// DefaultStyles, modify any fields, then pass to tui.New via tui.WithStyles.
type Styles struct {
	// Title styles the method name displayed in the form breadcrumb.
	Title lipgloss.Style
	// Subtitle styles the method description displayed below the title.
	Subtitle lipgloss.Style
	// Error styles error messages shown in forms and the response view.
	Error lipgloss.Style
	// Response styles the body of the response view.
	Response lipgloss.Style
	// FieldLabel styles the label text above each form field.
	FieldLabel lipgloss.Style
	// Required styles the asterisk marker on required fields.
	Required lipgloss.Style
	// Help styles keyboard-shortcut hints shown at the bottom of screens.
	Help lipgloss.Style
	// ActiveTab styles the currently-selected service tab in the tab bar.
	ActiveTab lipgloss.Style
	// InactiveTab styles unselected service tabs in the tab bar.
	InactiveTab lipgloss.Style
	// ToggleOn styles a toggleControl that is checked (true).
	ToggleOn lipgloss.Style
	// ToggleOff styles a toggleControl that is unchecked (false).
	ToggleOff lipgloss.Style
	// ToggleFocus styles a toggleControl that currently has focus.
	ToggleFocus lipgloss.Style
	// ListItem styles confirmed items in a listControl (repeated field inputs).
	ListItem lipgloss.Style
	// DateSegment styles inactive segments in date/time picker controls.
	DateSegment lipgloss.Style
	// DateActiveSegment styles the selected segment in date/time picker controls.
	DateActiveSegment lipgloss.Style
	// JSONValid styles the "✓ valid" indicator in the JSON editor.
	JSONValid lipgloss.Style
	// JSONInvalid styles parse-error indicators in the JSON editor.
	JSONInvalid lipgloss.Style
	// FocusedControl styles the border box drawn around the focused form field.
	FocusedControl lipgloss.Style
	// Control styles the border box drawn around unfocused form fields.
	Control lipgloss.Style
	// Modal styles the border box of modal overlay dialogs (border shape and
	// padding only — no border color, which is applied at render time).
	Modal lipgloss.Style
	// CardBox styles the outer border box of card grid entries (border shape
	// and padding only — no border color or width, which are applied at render
	// time based on focus state).
	CardBox lipgloss.Style
	// Colors holds the raw color tokens from the originating Theme. Use these
	// when building new lipgloss styles inside custom bubbles so that your
	// colors stay consistent with the rest of the TUI without needing to
	// extract them indirectly via GetForeground.
	Colors ThemeColors
	// FormIndent is the number of character cells used to indent form field
	// labels and controls from the left edge. Derived from Theme.Spacing.SM.
	FormIndent int
	// Custom stores user-defined styles registered via Theme.Custom.
	// Retrieve a style by name with the Get helper.
	Custom map[string]lipgloss.Style
}

// Get retrieves a custom style by name. Returns an empty lipgloss.Style if
// the name was not registered in the originating Theme.Custom map.
func (s Styles) Get(name string) lipgloss.Style {
	if s.Custom == nil {
		return lipgloss.NewStyle()
	}
	if style, ok := s.Custom[name]; ok {
		return style
	}
	return lipgloss.NewStyle()
}

// ThemeColors holds the named color palette that drives the full style set.
// Analogous to the color tokens in a Tailwind CSS theme configuration.
type ThemeColors struct {
	// Primary is used for active/focused elements: selected tabs, focused
	// segments, toggle-on state, list items, and focused control borders.
	Primary lipgloss.Color
	// Secondary is used for muted/inactive elements: inactive tabs, help text,
	// toggle-off state, unfocused date segments, subtitles, and placeholder text.
	Secondary lipgloss.Color
	// Accent is used for field labels and the toggle focus state.
	Accent lipgloss.Color
	// Error is used for error messages and required-field markers.
	Error lipgloss.Color
	// Success is used for the "✓ valid" indicator in validation controls.
	Success lipgloss.Color
	// Border is used for the border of unfocused form controls.
	// Typically a slightly darker shade than Secondary.
	Border lipgloss.Color
	// SecondaryBorder is used for lower-emphasis chrome such as card grid
	// borders and response table borders. Keeping it distinct from Border lets
	// you give cards a lighter visual weight than form controls.
	SecondaryBorder lipgloss.Color
	// Background is used for the whitespace fill behind modal overlays.
	// Defaults to terminal color 0 (black). Set to a dark neutral to create a
	// dimming effect without a full black overlay.
	Background lipgloss.Color
}

// ThemeSpacing holds character-width spacing tokens analogous to Tailwind's
// spacing scale. Values represent character cells (columns or rows).
// The defaults map Tailwind's relative proportions to terminal units:
// None=0, XS=1, SM=2, MD=4, LG=6, XL=8.
type ThemeSpacing struct {
	// None is always zero. Provided for explicit no-spacing use in Padding calls.
	None int
	// XS (extra-small) is 1 character. Used for horizontal control padding.
	XS int
	// SM (small) is 2 characters. Used for tab horizontal padding, modal
	// vertical padding, and the form content indent.
	SM int
	// MD (medium) is 4 characters. Used for modal horizontal padding.
	MD int
	// LG (large) is 6 characters.
	LG int
	// XL (extra-large) is 8 characters.
	XL int
}

// ThemeBorder holds lipgloss border styles for different UI contexts.
// Swap individual fields to change border shapes globally (e.g. NormalBorder
// instead of RoundedBorder) without touching colors or spacing.
type ThemeBorder struct {
	// Control is the border used for focused and unfocused form field boxes.
	Control lipgloss.Border
	// Card is the border used for card grid entries and response table chrome.
	Card lipgloss.Border
	// Modal is the border used for modal overlay boxes.
	Modal lipgloss.Border
}

// Theme is the comprehensive styling configuration for the TUI.
// It groups colors, spacing tokens, border shapes, and optional custom styles
// under a single value, following the same theming conventions as Tailwind CSS.
//
// Always start from DefaultTheme and modify the fields you need, rather than
// constructing a Theme literal from scratch. Constructing a zero-value Theme{}
// leaves Spacing and Border unset, which produces invisible borders and zero
// padding. StylesFromTheme applies safe fallbacks for Spacing and Border, but
// Colors are taken as-is, so unset colors render with terminal defaults.
//
// The recommended pattern for a color-only retheme:
//
//	tui.New(tui.WithTheme(bubbles.DefaultTheme().WithColors(bubbles.ThemeColors{
//	    Primary:   lipgloss.Color("205"),
//	    Secondary: lipgloss.Color("241"),
//	    Accent:    lipgloss.Color("213"),
//	    Error:     lipgloss.Color("196"),
//	    Success:   lipgloss.Color("2"),
//	    Border:    lipgloss.Color("238"),
//	    SecondaryBorder: lipgloss.Color("238"),
//	    Background:      lipgloss.Color("0"),
//	})))
//
// Custom styles can be registered in Theme.Custom and are forwarded into
// Styles.Custom, making them accessible to custom bubbles via Styles.Get:
//
//	theme := bubbles.DefaultTheme()
//	theme.Custom = map[string]lipgloss.Style{
//	    "badge": lipgloss.NewStyle().Bold(true).Background(lipgloss.Color("57")),
//	}
//	tui.New(tui.WithTheme(theme))
//
//	// Inside a custom bubble:
//	badge := styles.Get("badge").Render("NEW")
type Theme struct {
	// Colors drives the named color palette for all derived styles.
	Colors ThemeColors
	// Spacing defines character-width spacing tokens for padding and layout
	// throughout the TUI.
	Spacing ThemeSpacing
	// Border defines the border shapes used in different UI contexts.
	Border ThemeBorder
	// Custom stores additional user-defined styles keyed by an arbitrary name.
	// They are copied into Styles.Custom and accessible via Styles.Get so that
	// custom FormControl and ResponseView implementations can follow the
	// application's visual language without hardcoding styles.
	Custom map[string]lipgloss.Style
}

// WithColors returns a copy of the theme with the color palette replaced.
// This is the recommended shorthand for the common case of changing only colors:
//
//	tui.New(tui.WithTheme(bubbles.DefaultTheme().WithColors(myColors)))
func (t Theme) WithColors(c ThemeColors) Theme {
	t.Colors = c
	return t
}

// DefaultTheme returns the built-in theme: a purple/orange palette, Tailwind-
// proportioned spacing, and rounded borders throughout.
func DefaultTheme() Theme {
	return Theme{
		Colors: ThemeColors{
			Primary:         lipgloss.Color("62"),
			Secondary:       lipgloss.Color("241"),
			Accent:          lipgloss.Color("214"),
			Error:           lipgloss.Color("196"),
			Success:         lipgloss.Color("2"),
			Border:          lipgloss.Color("238"),
			SecondaryBorder: lipgloss.Color("238"),
			Background:      lipgloss.Color("0"),
		},
		Spacing: ThemeSpacing{
			None: 0,
			XS:   1,
			SM:   2,
			MD:   4,
			LG:   6,
			XL:   8,
		},
		Border: ThemeBorder{
			Control: lipgloss.RoundedBorder(),
			Card:    lipgloss.RoundedBorder(),
			Modal:   lipgloss.RoundedBorder(),
		},
	}
}

// StylesFromTheme derives a complete Styles set from a Theme.
// Colors, spacing tokens, and border shapes are all applied; Theme.Custom is
// forwarded verbatim into Styles.Custom. Use tui.WithStyleOverride afterwards
// to adjust any individual style beyond what the theme captures.
//
// Safe fallbacks are applied for zero-value Spacing and Border fields so that
// a partially-initialised Theme (e.g. constructed as a literal with only Colors
// set) produces a usable layout rather than invisible borders and zero padding.
func StylesFromTheme(t Theme) Styles {
	d := DefaultTheme()

	// Apply fallbacks for Spacing — all fields being zero means the caller
	// didn't set spacing (zero is an unusable value for most tokens).
	if t.Spacing == (ThemeSpacing{}) {
		t.Spacing = d.Spacing
	}

	// Apply per-field fallbacks for Border — an empty Top string is the
	// reliable indicator that a border shape hasn't been configured.
	if t.Border.Control.Top == "" {
		t.Border.Control = d.Border.Control
	}
	if t.Border.Card.Top == "" {
		t.Border.Card = d.Border.Card
	}
	if t.Border.Modal.Top == "" {
		t.Border.Modal = d.Border.Modal
	}

	c := t.Colors
	sp := t.Spacing
	br := t.Border
	return Styles{
		Title:             lipgloss.NewStyle().Bold(true).Foreground(c.Primary),
		Subtitle:          lipgloss.NewStyle().Foreground(c.Secondary),
		Error:             lipgloss.NewStyle().Foreground(c.Error),
		Response:          lipgloss.NewStyle().Bold(true),
		FieldLabel:        lipgloss.NewStyle().Foreground(c.Accent),
		Required:          lipgloss.NewStyle().Foreground(c.Error),
		Help:              lipgloss.NewStyle().Foreground(c.Secondary).Italic(true),
		ActiveTab:         lipgloss.NewStyle().Bold(true).Foreground(c.Primary).Padding(sp.None, sp.SM),
		InactiveTab:       lipgloss.NewStyle().Foreground(c.Secondary).Padding(sp.None, sp.SM),
		ToggleOn:          lipgloss.NewStyle().Bold(true).Foreground(c.Primary),
		ToggleOff:         lipgloss.NewStyle().Foreground(c.Secondary),
		ToggleFocus:       lipgloss.NewStyle().Foreground(c.Accent),
		ListItem:          lipgloss.NewStyle().Bold(true).Foreground(c.Primary),
		DateSegment:       lipgloss.NewStyle().Foreground(c.Secondary),
		DateActiveSegment: lipgloss.NewStyle().Bold(true).Foreground(c.Primary),
		JSONValid:         lipgloss.NewStyle().Foreground(c.Success),
		JSONInvalid:       lipgloss.NewStyle().Foreground(c.Error),
		FocusedControl: lipgloss.NewStyle().
			Border(br.Control).
			BorderForeground(c.Primary).
			Padding(sp.None, sp.XS),
		Control: lipgloss.NewStyle().
			Border(br.Control).
			BorderForeground(c.Border).
			Padding(sp.None, sp.XS),
		Modal: lipgloss.NewStyle().
			Border(br.Modal).
			Padding(sp.SM, sp.MD).
			Align(lipgloss.Center),
		CardBox: lipgloss.NewStyle().
			Border(br.Card).
			Padding(sp.None, sp.XS),
		Colors:     t.Colors,
		FormIndent: sp.SM,
		Custom:     t.Custom,
	}
}

// DefaultStyles returns the built-in style set derived from DefaultTheme.
// Use this as a starting point when you need to adjust individual styles
// beyond what a theme swap covers.
func DefaultStyles() Styles {
	return StylesFromTheme(DefaultTheme())
}
