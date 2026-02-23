package bubbles

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

// ─── Shared helpers ───────────────────────────────────────────────────────────

// daysInMonth returns the number of days in the given month for the given year,
// correctly accounting for leap years.
func daysInMonth(year, month int) int {
	return time.Date(year, time.Month(month)+1, 0, 0, 0, 0, 0, time.UTC).Day()
}

// clampInt returns v clamped to [lo, hi].
func clampInt(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// segDisplay renders a single date/time segment value for display. When active
// and a digit buffer is being built, it shows the buffer padded with "_" to the
// right so the user can see how many more digits are expected.
func segDisplay(val, width int, buf string, active bool, styles Styles) string {
	var text string
	if active && len(buf) > 0 {
		text = buf + strings.Repeat("_", width-len(buf))
	} else {
		text = fmt.Sprintf("%0*d", width, val)
	}
	if active {
		return styles.DateActiveSegment.Render(text)
	}
	return styles.DateSegment.Render(text)
}

// ─── Date picker ──────────────────────────────────────────────────────────────

type dateSeg int

const (
	dsYear  dateSeg = iota
	dsMonth dateSeg = iota
	dsDay   dateSeg = iota
)

// dateControl is an interactive segment-by-segment date picker.
//
// Navigation:
//   - ←/→ move between year / month / day segments
//   - ↑/↓ increment or decrement the active segment
//   - 0–9 type digits directly; the segment auto-advances when full
//   - Backspace removes the last typed digit
type dateControl struct {
	year, month, day int
	seg              dateSeg
	digitBuf         string
	focused          bool
	styles           Styles
}

// NewDateControl returns a date picker FormControl.
// defaultValue must be in "YYYY-MM-DD" format; today's local date is used
// when it is empty or unparseable.
func NewDateControl(defaultValue string, styles Styles) FormControl {
	now := time.Now()
	year, month, day := now.Year(), int(now.Month()), now.Day()
	if defaultValue != "" {
		if t, err := time.Parse("2006-01-02", defaultValue); err == nil {
			year, month, day = t.Year(), int(t.Month()), t.Day()
		}
	}
	return &dateControl{year: year, month: month, day: day, styles: styles}
}

// CapturesKey implements KeyCapturer. The date picker uses +/- for segment
// changes, so no keys need to be captured from form navigation.
func (c *dateControl) CapturesKey(key string) bool {
	return false
}

func (c *dateControl) Focus() tea.Cmd { c.focused = true; return nil }
func (c *dateControl) Blur() {
	c.focused = false
	c.digitBuf = ""
}

// Value returns the date as "YYYY-MM-DD".
func (c *dateControl) Value() string {
	return fmt.Sprintf("%04d-%02d-%02d", c.year, c.month, c.day)
}

func (c *dateControl) View() string {
	yr := segDisplay(c.year, 4, c.digitBuf, c.focused && c.seg == dsYear, c.styles)
	mo := segDisplay(c.month, 2, c.digitBuf, c.focused && c.seg == dsMonth, c.styles)
	dy := segDisplay(c.day, 2, c.digitBuf, c.focused && c.seg == dsDay, c.styles)
	line := yr + "  -  " + mo + "  -  " + dy
	if c.focused {
		line += "\n" + c.styles.Help.Render("←→: field  +/-: change  0-9: type")
	}
	return line
}

func (c *dateControl) Update(msg tea.Msg) tea.Cmd {
	key, ok := msg.(tea.KeyMsg)
	if !ok {
		return nil
	}
	switch key.String() {
	case "left":
		c.digitBuf = ""
		if c.seg > dsYear {
			c.seg--
		}
	case "right":
		c.digitBuf = ""
		if c.seg < dsDay {
			c.seg++
		}
	case "+":
		c.digitBuf = ""
		c.adjustDateSeg(+1)
	case "-":
		c.digitBuf = ""
		c.adjustDateSeg(-1)
	case "backspace":
		if len(c.digitBuf) > 0 {
			c.digitBuf = c.digitBuf[:len(c.digitBuf)-1]
		}
	default:
		if len(key.String()) == 1 && key.String()[0] >= '0' && key.String()[0] <= '9' {
			c.typeDateDigit(int(key.String()[0] - '0'))
		}
	}
	return nil
}

func (c *dateControl) typeDateDigit(d int) {
	maxLen := 2
	if c.seg == dsYear {
		maxLen = 4
	}
	if len(c.digitBuf) >= maxLen {
		c.digitBuf = "" // restart entry
	}
	c.digitBuf += string(rune('0' + d))
	v, _ := strconv.Atoi(c.digitBuf)
	switch c.seg {
	case dsYear:
		c.year = clampInt(v, 1, 9999)
	case dsMonth:
		if v > 12 {
			v = 12
		}
		c.month = clampInt(v, 1, 12)
		c.day = clampInt(c.day, 1, daysInMonth(c.year, c.month))
	case dsDay:
		maxDay := daysInMonth(c.year, c.month)
		c.day = clampInt(v, 1, maxDay)
	}
	if len(c.digitBuf) >= maxLen {
		c.digitBuf = ""
		if c.seg < dsDay {
			c.seg++
		}
	}
}

func (c *dateControl) adjustDateSeg(delta int) {
	switch c.seg {
	case dsYear:
		c.year = clampInt(c.year+delta, 1, 9999)
		c.day = clampInt(c.day, 1, daysInMonth(c.year, c.month))
	case dsMonth:
		c.month += delta
		if c.month < 1 {
			c.month = 12
		} else if c.month > 12 {
			c.month = 1
		}
		c.day = clampInt(c.day, 1, daysInMonth(c.year, c.month))
	case dsDay:
		maxDay := daysInMonth(c.year, c.month)
		c.day += delta
		if c.day < 1 {
			c.day = maxDay
		} else if c.day > maxDay {
			c.day = 1
		}
	}
}

// ─── Date-time picker ─────────────────────────────────────────────────────────

// TimestampInputTZ controls how values entered in a date-time picker are interpreted.
type TimestampInputTZ int

const (
	// UTCTimezone interprets entered values as UTC (default).
	UTCTimezone TimestampInputTZ = iota
	// SystemTimezone interprets entered values in the system's local timezone.
	SystemTimezone
	// UserProvidedTimezone adds an editable timezone field to the picker.
	// The user can type any IANA name (e.g. "America/New_York") or a
	// numeric offset (e.g. "+05:30").
	UserProvidedTimezone
)

// TimestampNormalization controls how Value() formats the resulting RFC3339 string.
type TimestampNormalization int

const (
	// ToUTC normalises Value() to a UTC RFC3339 string (default).
	ToUTC TimestampNormalization = iota
	// RetainSourceTimezone formats Value() with the source timezone offset,
	// preserving that offset in the RFC3339 output.
	RetainSourceTimezone
)

// DateTimeControlOption configures a DateTimeControl created via NewDateTimeControl.
type DateTimeControlOption func(*dateTimeControl)

// WithInputTimezone sets the timezone that date-time values entered in the
// picker represent. Defaults to UTCTimezone.
func WithInputTimezone(tz TimestampInputTZ) DateTimeControlOption {
	return func(c *dateTimeControl) { c.inputTZ = tz }
}

// WithTZNormalization sets how Value() formats the resulting RFC3339 timestamp.
// Defaults to ToUTC.
func WithTZNormalization(n TimestampNormalization) DateTimeControlOption {
	return func(c *dateTimeControl) { c.normalization = n }
}

type dtSeg int

const (
	dtYear   dtSeg = iota
	dtMonth  dtSeg = iota
	dtDay    dtSeg = iota
	dtHour   dtSeg = iota
	dtMinute dtSeg = iota
	dtSecond dtSeg = iota
)

// dateTimeControl is an interactive segment-by-segment date+time picker.
// It returns an RFC3339 string suitable for google.protobuf.Timestamp fields
// (via tui.WithTimestampControl). The output timezone is determined by the
// normalization option; the input interpretation is determined by inputTZ.
//
// Navigation: ←/→ move between date/time segments (and to the timezone field
// when UserProvidedTimezone is configured), ↑/↓ increment/decrement the active
// segment, and digit keys type directly.
type dateTimeControl struct {
	year, month, day int
	hour, min, sec   int
	seg              dtSeg
	digitBuf         string
	focused          bool
	styles           Styles

	inputTZ       TimestampInputTZ
	normalization TimestampNormalization
	tzInput       textinput.Model // only used when inputTZ == UserProvidedTimezone
	tzFocused     bool            // true when the timezone text field has focus
}

// NewDateTimeControl returns a date+time picker FormControl.
// defaultValue must be an RFC3339 string; the current time in the input
// timezone is used when it is empty or unparseable.
func NewDateTimeControl(defaultValue string, styles Styles, opts ...DateTimeControlOption) FormControl {
	c := &dateTimeControl{styles: styles}
	for _, opt := range opts {
		opt(c)
	}

	// Initialise the timezone text input before calling inputLoc().
	if c.inputTZ == UserProvidedTimezone {
		ti := textinput.New()
		ti.Placeholder = "UTC, America/New_York, +05:30 …"
		ti.Width = 30
		c.tzInput = ti
		c.tzInput.SetValue("UTC")
	}

	now := time.Now().In(c.inputLoc())
	c.year, c.month, c.day = now.Year(), int(now.Month()), now.Day()
	c.hour, c.min, c.sec = now.Hour(), now.Minute(), now.Second()

	if defaultValue != "" {
		if t, err := time.Parse(time.RFC3339, defaultValue); err == nil {
			t = t.In(c.inputLoc())
			c.year, c.month, c.day = t.Year(), int(t.Month()), t.Day()
			c.hour, c.min, c.sec = t.Hour(), t.Minute(), t.Second()
		}
	}

	return c
}

// CapturesKey implements KeyCapturer. The date-time picker uses +/- for
// segment changes, so only ← (to leave the timezone sub-field) needs to be
// captured; up/down are free for normal form field navigation.
func (c *dateTimeControl) CapturesKey(key string) bool {
	return c.tzFocused && key == "left"
}

func (c *dateTimeControl) Focus() tea.Cmd {
	c.focused = true
	return nil
}

func (c *dateTimeControl) Blur() {
	c.focused = false
	c.digitBuf = ""
	c.tzFocused = false
	if c.inputTZ == UserProvidedTimezone {
		c.tzInput.Blur()
	}
}

// Value returns an RFC3339 string. When normalization is ToUTC (default) the
// output is in UTC; RetainSourceTimezone preserves the input timezone offset.
func (c *dateTimeControl) Value() string {
	t := time.Date(c.year, time.Month(c.month), c.day, c.hour, c.min, c.sec, 0, c.inputLoc())
	if c.normalization == RetainSourceTimezone {
		return t.Format(time.RFC3339)
	}
	return t.UTC().Format(time.RFC3339)
}

func (c *dateTimeControl) View() string {
	buf := c.digitBuf
	active := c.focused && !c.tzFocused

	yr := segDisplay(c.year, 4, buf, active && c.seg == dtYear, c.styles)
	mo := segDisplay(c.month, 2, buf, active && c.seg == dtMonth, c.styles)
	dy := segDisplay(c.day, 2, buf, active && c.seg == dtDay, c.styles)
	hr := segDisplay(c.hour, 2, buf, active && c.seg == dtHour, c.styles)
	mn := segDisplay(c.min, 2, buf, active && c.seg == dtMinute, c.styles)
	sc := segDisplay(c.sec, 2, buf, active && c.seg == dtSecond, c.styles)

	date := yr + "  -  " + mo + "  -  " + dy
	clock := hr + "  :  " + mn + "  :  " + sc
	line := date + "     " + clock + "  " + c.viewTZ()

	if c.focused {
		if c.tzFocused {
			line += "\n" + c.styles.Help.Render("←: back to date/time  •  Tab: next field")
		} else {
			hint := "←→: field  +/-: change  0-9: type"
			if c.inputTZ == UserProvidedTimezone {
				hint += "  →: edit timezone"
			}
			line += "\n" + c.styles.Help.Render(hint)
		}
	}
	return line
}

// viewTZ renders the timezone portion of the date-time display.
func (c *dateTimeControl) viewTZ() string {
	switch c.inputTZ {
	case SystemTimezone:
		name, _ := time.Now().Zone()
		return c.styles.DateSegment.Render(name)
	case UserProvidedTimezone:
		if c.focused && c.tzFocused {
			return c.tzInput.View()
		}
		val := strings.TrimSpace(c.tzInput.Value())
		if val == "" {
			val = "UTC"
		}
		return c.styles.DateSegment.Render(val)
	default: // UTCTimezone
		return c.styles.DateSegment.Render("UTC")
	}
}

func (c *dateTimeControl) Update(msg tea.Msg) tea.Cmd {
	key, ok := msg.(tea.KeyMsg)
	if !ok {
		return nil
	}

	// Timezone field active — delegate all keys; ← returns to datetime segments.
	if c.tzFocused {
		if key.String() == "left" {
			c.tzFocused = false
			c.tzInput.Blur()
			return nil
		}
		var cmd tea.Cmd
		c.tzInput, cmd = c.tzInput.Update(msg)
		return cmd
	}

	// Normal datetime segment handling.
	switch key.String() {
	case "left":
		c.digitBuf = ""
		if c.seg > dtYear {
			c.seg--
		}
	case "right":
		c.digitBuf = ""
		if c.seg < dtSecond {
			c.seg++
		} else if c.inputTZ == UserProvidedTimezone {
			c.tzFocused = true
			return c.tzInput.Focus()
		}
	case "+":
		c.digitBuf = ""
		c.adjustDTSeg(+1)
	case "-":
		c.digitBuf = ""
		c.adjustDTSeg(-1)
	case "backspace":
		if len(c.digitBuf) > 0 {
			c.digitBuf = c.digitBuf[:len(c.digitBuf)-1]
		}
	default:
		if len(key.String()) == 1 && key.String()[0] >= '0' && key.String()[0] <= '9' {
			c.typeDTDigit(int(key.String()[0] - '0'))
		}
	}
	return nil
}

// inputLoc returns the *time.Location used to interpret entered date-time values.
func (c *dateTimeControl) inputLoc() *time.Location {
	switch c.inputTZ {
	case SystemTimezone:
		return time.Local
	case UserProvidedTimezone:
		if loc, err := parseLocation(c.tzInput.Value()); err == nil {
			return loc
		}
		return time.UTC
	default:
		return time.UTC
	}
}

// parseLocation parses an IANA timezone name (e.g. "America/New_York") or a
// numeric offset (e.g. "+05:30", "-0800") into a *time.Location.
func parseLocation(s string) (*time.Location, error) {
	s = strings.TrimSpace(s)
	if s == "" || strings.EqualFold(s, "utc") || s == "Z" || s == "z" {
		return time.UTC, nil
	}
	if strings.EqualFold(s, "local") {
		return time.Local, nil
	}
	if loc, err := time.LoadLocation(s); err == nil {
		return loc, nil
	}
	for _, layout := range []string{"-07:00", "-0700"} {
		if t, err := time.Parse(layout, s); err == nil {
			_, offset := t.Zone()
			return time.FixedZone(s, offset), nil
		}
	}
	return nil, fmt.Errorf("unrecognized timezone %q: use an IANA name (e.g. America/New_York) or offset (e.g. +05:30)", s)
}

func (c *dateTimeControl) typeDTDigit(d int) {
	maxLen := 2
	if c.seg == dtYear {
		maxLen = 4
	}
	if len(c.digitBuf) >= maxLen {
		c.digitBuf = ""
	}
	c.digitBuf += string(rune('0' + d))
	v, _ := strconv.Atoi(c.digitBuf)
	switch c.seg {
	case dtYear:
		c.year = clampInt(v, 1, 9999)
	case dtMonth:
		if v > 12 {
			v = 12
		}
		c.month = clampInt(v, 1, 12)
		c.day = clampInt(c.day, 1, daysInMonth(c.year, c.month))
	case dtDay:
		maxDay := daysInMonth(c.year, c.month)
		c.day = clampInt(v, 1, maxDay)
	case dtHour:
		if v > 23 {
			v = 23
		}
		c.hour = clampInt(v, 0, 23)
	case dtMinute:
		if v > 59 {
			v = 59
		}
		c.min = clampInt(v, 0, 59)
	case dtSecond:
		if v > 59 {
			v = 59
		}
		c.sec = clampInt(v, 0, 59)
	}
	if len(c.digitBuf) >= maxLen {
		c.digitBuf = ""
		if c.seg < dtSecond {
			c.seg++
		}
	}
}

func (c *dateTimeControl) adjustDTSeg(delta int) {
	switch c.seg {
	case dtYear:
		c.year = clampInt(c.year+delta, 1, 9999)
		c.day = clampInt(c.day, 1, daysInMonth(c.year, c.month))
	case dtMonth:
		c.month += delta
		if c.month < 1 {
			c.month = 12
		} else if c.month > 12 {
			c.month = 1
		}
		c.day = clampInt(c.day, 1, daysInMonth(c.year, c.month))
	case dtDay:
		maxDay := daysInMonth(c.year, c.month)
		c.day += delta
		if c.day < 1 {
			c.day = maxDay
		} else if c.day > maxDay {
			c.day = 1
		}
	case dtHour:
		c.hour = (c.hour + delta + 24) % 24
	case dtMinute:
		c.min = (c.min + delta + 60) % 60
	case dtSecond:
		c.sec = (c.sec + delta + 60) % 60
	}
}
